# Update CloudFormation Stack Parameter Action

This GitHub Action updates a single parameter in an existing CloudFormation stack while preserving all other settings.

## Usage

```yaml
- uses: artyom/update-cloudformation-stack@main
  with:
    stack: my-stack-name
    key: ParameterName
    value: NewValue
```

## Inputs

- `stack` - Name of the CloudFormation stack to update
- `key` - Name of the stack parameter to update
- `value` - New value to set for the parameter

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
          key: ImageTag
          value: v123
```

The action will monitor stack update progress and fail if update fails. If parameter already has the requested value, action will exit with a warning message.
