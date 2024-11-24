# Update CloudFormation Stack Parameters Action

This GitHub Action updates existing CloudFormation stack by changing some of its parameters while preserving all other settings.

## Usage

```yaml
- uses: artyom/update-cloudformation-stack@main
  with:
    stack: my-stack-name
    parameters: |
      Name1=value1
      Name2=value2
```

## Inputs

- `stack` - name of the CloudFormation stack to update
- `parameters` - pairs of parameters in the Name=Value format, each pair on a separate line

## AWS Credentials

This action uses the AWS SDK default credential provider chain. Configure AWS credentials using standard GitHub Actions methods:

```yaml
- uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: arn:aws:iam::123456789012:role/my-role
    aws-region: us-east-1
```

## AWS Permissions

This action requires the following permissions:

- cloudformation:DescribeStacks
- cloudformation:UpdateStack
- cloudformation:DescribeStackEvents

## Example

```yaml
jobs:
  update-stack:
    runs-on: ubuntu-latest
    steps:
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/my-role
          aws-region: us-east-1
      - uses: artyom/update-cloudformation-stack@main
        with:
          stack: production-stack
          parameters: |
            ImageTag=v123
```

The action will monitor stack update progress and fail if update fails.
