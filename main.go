package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
)

func main() {
	log.SetFlags(0)
	var args runArgs
	flag.StringVar(&args.stack, "stack", args.stack, "name of the CloudFormation stack to update")
	flag.StringVar(&args.key, "key", args.key, "parameter name to update")
	flag.StringVar(&args.value, "value", args.value, "parameter value to set")
	flag.Parse()
	if err := run(context.Background(), args); err != nil {
		if errors.Is(err, errAlreadySet) {
			log.Print(githubWarnPrefix, err)
			return
		}
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ValidationError" && ae.ErrorMessage() == "No updates are to be performed." {
			log.Print(githubWarnPrefix, "nothing to update")
			return
		}
		log.Fatal(githubErrPrefix, err)
	}
}

type runArgs struct {
	stack string
	key   string
	value string
}

var errAlreadySet = errors.New("stack already has required parameter value")

func run(ctx context.Context, args runArgs) error {
	if err := args.validate(); err != nil {
		return err
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	svc := cloudformation.NewFromConfig(cfg)

	desc, err := svc.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &args.stack})
	if err != nil {
		return err
	}
	if l := len(desc.Stacks); l != 1 {
		return fmt.Errorf("DescribeStacks returned %d stacks, expected 1", l)
	}
	stack := desc.Stacks[0]
	var params []types.Parameter
	var seenKey bool
	for _, p := range stack.Parameters {
		k := aws.ToString(p.ParameterKey)
		if k == args.key && aws.ToString(p.ParameterValue) == args.value {
			return errAlreadySet
		}
		if k == args.key {
			seenKey = true
			continue
		}
		params = append(params, types.Parameter{ParameterKey: &k, UsePreviousValue: aws.Bool(true)})
	}
	if !seenKey {
		return errors.New("stack has no parameter with the given key")
	}
	params = append(params, types.Parameter{ParameterKey: &args.key, ParameterValue: &args.value})

	token := newToken()
	_, err = svc.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:           &args.stack,
		ClientRequestToken:  &token,
		UsePreviousTemplate: aws.Bool(true),
		Parameters:          params,
		Capabilities:        stack.Capabilities,
		NotificationARNs:    stack.NotificationARNs,
	})
	if err != nil {
		return err
	}
	log.Print("polling for stack updates until it's ready, this may take a while")
	debugf := func(format string, args ...any) {
		if !underGithub {
			return
		}
		log.Printf("::debug::"+format, args...)
	}
	oldEventsCutoff := time.Now().Add(-time.Hour)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
		p := cloudformation.NewDescribeStackEventsPaginator(svc, &cloudformation.DescribeStackEventsInput{StackName: &args.stack})
	scanEvents:
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return err
			}
			for _, evt := range page.StackEvents {
				if evt.Timestamp != nil && aws.ToTime(evt.Timestamp).Before(oldEventsCutoff) {
					break scanEvents
				}
				if evt.ClientRequestToken == nil || *evt.ClientRequestToken != token {
					continue
				}
				if evt.ResourceStatus == types.ResourceStatusUpdateFailed {
					return fmt.Errorf("%v: %s", evt.ResourceStatus, aws.ToString(evt.ResourceStatusReason))
				}
				debugf("%s\t%s\t%v", aws.ToString(evt.ResourceType), aws.ToString(evt.LogicalResourceId), evt.ResourceStatus)
				if aws.ToString(evt.LogicalResourceId) == args.stack && aws.ToString(evt.ResourceType) == "AWS::CloudFormation::Stack" {
					switch evt.ResourceStatus {
					case types.ResourceStatusUpdateComplete:
						return nil
					}
				}
			}
		}
	}
}

func newToken() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "ucs-" + hex.EncodeToString(b)
}

func (a *runArgs) validate() error {
	if a.stack == "" || a.key == "" || a.value == "" {
		return errors.New("stack, key, and value cannot be empty")
	}
	return nil
}

func init() {
	const usage = `Updates a single parameter in an existing CloudFormation stack while preserving all other settings.

Usage: update-cloudformation-stack -stack NAME -key PARAM -value VALUE
`
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
}

var underGithub bool
var githubWarnPrefix string
var githubErrPrefix string

func init() {
	underGithub = os.Getenv("GITHUB_ACTIONS") == "true"
	if underGithub {
		githubWarnPrefix = "::warning::"
		githubErrPrefix = "::error::"
	}
}
