# SpartaXForm

Sparta-based application that provisions a Kinesis Firehose that includes an
[AWS Lambda Transformer](https://aws.amazon.com/blogs/compute/amazon-kinesis-firehose-data-transformation-with-aws-lambda/) function.

The transformation is specified by a `go` _text/template_ value that
supports the default Kinesis Firehose test data format:

```json
{
  "ticker_symbol": "QXZ",
  "sector": "HEALTHCARE",
  "change": -0.05,
  "price": 84.51
}
```

Transformation:

```text
{
    "region" : "{{ .Record.KinesisEventHeader.Region  }}",
    "key" : {{ .Record.Data.JMESPath "sector"}}
}
```

See the [documentation](http://gosparta.io/reference/archetypes/kinesis_firehose/) for
more information.

## Instructions

1. [Install Go](https://golang.org/doc/install)
1. `go get github.com/mweagle/SpartaXForm`
1. `cd ./SpartaXForm`
1. `go run main.go provision --s3Bucket YOUR_S3_BUCKET`
1. Visit the AWS Kinesis Firehose Console and test your function!
