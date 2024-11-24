package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
)

func main() {
	log.SetFlags(0)
	var stackName string
	flag.StringVar(&stackName, "stack", stackName, "name of the CloudFormation stack to update")
	flag.Parse()
	if err := run(context.Background(), stackName, flag.Args()); err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ValidationError" && ae.ErrorMessage() == "No updates are to be performed." {
			log.Print(githubWarnPrefix, "nothing to update")
			return
		}
		log.Fatal(githubErrPrefix, err)
	}
}

func run(ctx context.Context, stackName string, args []string) error {
	if stackName == "" {
		return errors.New("stack name must be set")
	}
	if underGithub && len(args) == 0 {
		args = strings.Split(os.Getenv("INPUT_PARAMETERS"), "\n")
	}
	toReplace, err := parseKvs(args)
	if err != nil {
		return err
	}
	if len(toReplace) == 0 {
		return errors.New("empty parameters list")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	svc := cloudformation.NewFromConfig(cfg)

	desc, err := svc.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
	if err != nil {
		return err
	}
	if l := len(desc.Stacks); l != 1 {
		return fmt.Errorf("DescribeStacks returned %d stacks, expected 1", l)
	}
	stack := desc.Stacks[0]
	var params []types.Parameter
	for _, p := range stack.Parameters {
		k := aws.ToString(p.ParameterKey)
		if v, ok := toReplace[k]; ok {
			params = append(params, types.Parameter{ParameterKey: &k, ParameterValue: &v})
			delete(toReplace, k)
			continue
		}
		params = append(params, types.Parameter{ParameterKey: &k, UsePreviousValue: aws.Bool(true)})
	}
	if len(toReplace) != 0 {
		return fmt.Errorf("stack has no parameters with these names: %s", strings.Join(slices.Sorted(maps.Keys(toReplace)), ", "))
	}

	token := newToken()
	_, err = svc.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:           &stackName,
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
		p := cloudformation.NewDescribeStackEventsPaginator(svc, &cloudformation.DescribeStackEventsInput{StackName: &stackName})
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
				if aws.ToString(evt.LogicalResourceId) == stackName && aws.ToString(evt.ResourceType) == "AWS::CloudFormation::Stack" {
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

func init() {
	const usage = `Updates CloudFormation stack by updating some of its parameters while preserving all other settings.

Usage: update-cloudformation-stack -stack=NAME Param1=Value1 [Param2=Value2 ...]
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

func parseKvs(list []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, line := range list {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("wrong parameter format, want key=value pair: %q", line)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			return nil, fmt.Errorf("wrong parameter format, both key and value must be non-empty: %q", line)
		}
		if _, ok := out[k]; ok {
			return nil, fmt.Errorf("duplicate key in parameters list: %q", k)
		}
		out[k] = v
	}
	return out, nil
}
