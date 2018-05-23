/*
Package main is the main executable of the serverless function. It will query the Google
Calendar API and search for upcoming events of the user whose OAuth Token is used. For
each event a message will be sent to a Trello function to create a new Trello card
*/
package main

// The imports
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	rt "github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-xray-sdk-go/xray"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
)

// Variables that are set as Environment Variables
var (
	trelloARN            = os.Getenv("arntrello")
	clientSecret         = os.Getenv("cspointer")
	calendarTimeInterval = os.Getenv("interval")
	calendarTokenPointer = os.Getenv("tokenpointer")
	region               = "us-west-2"
	awsConfig            *aws.Config
	ssmSession           *ssm.SSM
)

type lambdaEvent struct {
	EventVersion string
	EventSource  string
	Trello       trelloEvent
}

type trelloEvent struct {
	Title       string
	Description string
}

const (
	// The date format used by Go
	dateFormat = "02/01/2006 15:04"
)

// The handler function is executed every time that a new Lambda event is received.
// It takes a JSON payload (you can see an example in the event.json file) and only
// returns an error if the something went wrong. The event comes fom CloudWatch and
// is scheduled every interval (where the interval is defined as variable)
func handler(request events.CloudWatchEvent) error {
	// Create a context
	ctx := context.Background()

	// Prepare AWS Configuration
	awsConfig = aws.NewConfig().WithRegion(region)
	xray.Configure(xray.Config{LogLevel: "trace"})
	ctx, seg := xray.BeginSegment(context.Background(), "gocal")
	ctx, subSegStart := xray.BeginSubsegment(ctx, "startup")
	initializeSSMSession()

	// stdout and stderr are sent to AWS CloudWatch Logs
	log.Printf("Processing Lambda request [%s]", request.ID)

	// Create a new Google configuration
	csString, err := getSSMParameter(ssmSession, clientSecret, true)
	if err != nil {
		log.Fatalf("Error trying to get parameter: %v", err)
	}
	byteString := []byte(csString)
	config, err := google.ConfigFromJSON(byteString, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	// Create a new HTTP client
	client := getClient(ctx, config)

	// Create a connection to Google Calendar
	srv, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve calendar Client %v", err)
	}

	// Generate timestamps for tomorrow and tomorrow + time interval
	i, _ := strconv.Atoi(calendarTimeInterval)
	tomorrow := time.Now().Add(time.Hour * 24)
	interval := time.Duration(i) * time.Minute
	timeStart := tomorrow.Format(time.RFC3339)
	timeEnd := tomorrow.Add(interval).Format(time.RFC3339)
	log.Printf("We will get calendar entries between %s and %s\n", timeStart, timeEnd)

	// Get the calendar entries
	events, err := srv.Events.List("primary").ShowDeleted(false).SingleEvents(true).TimeMin(timeStart).TimeMax(timeEnd).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve user's events. %v", err)
	}

	// Close the subsegment
	subSegStart.Close(nil)

	// Loop over the calendar events
	if len(events.Items) > 0 {
		// Create a new AWS session to invoke a Lambda function
		aws := lambda.New(session.New(awsConfig))
		xray.AWS(aws.Client)
		// Start subsegment lambda
		ctx, subSeg := xray.BeginSubsegment(ctx, "lambda")
		for _, i := range events.Items {
			var when string
			// If the DateTime is an empty string the Event is an all-day Event and those are ignored for now
			// So only Date is available.
			if i.Start.DateTime != "" {
				t, err := time.Parse(time.RFC3339, i.Start.DateTime)
				if err != nil {
					fmt.Println(err)
				}
				when = t.Format(dateFormat)

				payload := lambdaEvent{
					EventVersion: "1.0",
					EventSource:  "aws:lambda",
					Trello: trelloEvent{
						Title:       "M: (" + when + ") " + i.Summary,
						Description: i.Description,
					},
				}

				var b []byte
				b, _ = json.Marshal(payload)

				// Execute the call to the Trello Lambda function
				_, errLambda := aws.InvokeWithContext(ctx, &lambda.InvokeInput{
					FunctionName: &trelloARN,
					Payload:      b})

				if errLambda != nil {
					log.Printf(errLambda.Error())
					return errLambda
				}
				log.Printf("%s, %s\n%s\n", when, i.Summary, i.Description)
			}
			// Close the subsegment
			subSeg.Close(nil)
			seg.Close(nil)
		}
	} else {
		log.Printf("No upcoming events found.\n")
	}

	return nil
}

// The main method is executed by AWS Lambda and points to the handler
func main() {
	rt.Start(handler)
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	tok, err := tokenFromSSM()
	if err != nil {
		tok = getTokenFromWeb(config)
		putTokenInSSM(tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenFromSSM retrieves a Token from AWS SSM.
// It returns the retrieved Token and any read error encountered.
func tokenFromSSM() (*oauth2.Token, error) {
	f, err := getSSMParameter(ssmSession, calendarTokenPointer, true)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.Unmarshal([]byte(f), t)
	return t, err
}

// putTokenInSSM saves the token to AWS SSM
func putTokenInSSM(token *oauth2.Token) {
	f, err := json.Marshal(token)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}

	_, err = putSSMParameter(ssmSession, calendarTokenPointer, true, "SecureString", string(f))
	if err != nil {
		log.Fatalf("Unable to save oauth token: %v", err)
	}
}

// initializSSMSession creates an SSM session object and wraps it in Xray
func initializeSSMSession() {
	ssmSession = ssm.New(session.New(awsConfig))
}

// getSSMParameter gets a parameter from the AWS Simple Systems Manager service.
func getSSMParameter(ssmSession *ssm.SSM, name string, decrypt bool) (string, error) {
	gpi := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(decrypt),
	}

	param, err := ssmSession.GetParameter(gpi)
	if err != nil {
		return "", err
	}

	return *param.Parameter.Value, nil
}

// getSSMParameter puts a parameter in the AWS Simple Systems Manager service.
func putSSMParameter(ssmSession *ssm.SSM, name string, overwrite bool, paramtype string, value string) (int64, error) {
	ppi := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Overwrite: aws.Bool(overwrite),
		Type:      aws.String(paramtype),
		Value:     aws.String(value),
	}

	param, err := ssmSession.PutParameter(ppi)
	if err != nil {
		return -1, err
	}

	return *param.Version, nil
}
