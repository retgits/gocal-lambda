package main

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandler(t *testing.T) {
	t.Run("Successful Request", func(t *testing.T) {
		byteArray := []byte(`{"source": "aws.events","account": "123456789012","time": "1970-01-01T00:00:00Z","id": "cdc73f9d-aea9-11e3-9d5a-835b769c0d9c","region": "us-east-1","detail": {},"resources": ["arn:aws:events:us-east-1:123456789012:rule/my-schedule"],"detail-type": "Scheduled Event"}`)
		var datamap events.CloudWatchEvent
		if err := json.Unmarshal(byteArray, &datamap); err != nil {
			panic(err)
		}

		err := handler(datamap)
		if err != nil {
			t.Fatal("Everything should be ok")
		}
	})
}
