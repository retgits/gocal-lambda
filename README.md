# Lambda app for Google Calendar

This Serverless function is designed to query the Google Calendar API for upcoming appointments of the current user.

## Layout
```bash
.
├── build.sh                    <-- Make to automate build
├── event.json                  <-- Sample event to test using SAM local
├── README.md                   <-- This file
├── src                         <-- Source code for a lambda function
│   ├── main.go                 <-- Lambda function code
│   └── main_test.go            <-- Unit tests
└── template.yaml               <-- SAM Template
```

## build.sh
The `build.sh` file has seven commands to make working with this app easier than it already is

* deps: go get and update all the dependencies
* clean: removes the ./bin folder
* test: uses SAM local and the event in `event.json` to test the implementation
* build: creates the executable
* getparams: updates the SAM template with the values from the AWS Systems Manager Parameter Store
* delparams: removes the values of the environment variables in the SAM template
* deploy: deploy the function to AWS Lambda

## Prerequisites
While executing the the build script there are a few programs that are used:

* [jq](https://stedolan.github.io/jq/)
* [yq](https://github.com/mikefarah/yq)
* [aws cli](https://github.com/aws/aws-cli)
* [sam cli](https://github.com/awslabs/aws-sam-cli)

## AWS Systems Manager
Within the AWS Systems Manager Parameter store there are three parameters that are used in this app:

* /arn/trello
* /gocal*/clientsecret
* /gocal*/token
* /gocal*/interval
* /gocal*/tokenpointer
* /gocal*/cspointer

## TODO
- [ ] Update the `deps` target in build.sh to make use of dep or simply have a smarter approach than list all dependencies
- [ ] Make sure that all the calls to SSM are correctly traced with XRay
- [ ] Come up with a better way to deploy the same function with different parameters
