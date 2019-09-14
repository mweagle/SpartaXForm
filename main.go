package main

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	sparta "github.com/mweagle/Sparta"
	"github.com/mweagle/Sparta/archetype"
	spartaCF "github.com/mweagle/Sparta/aws/cloudformation"
	spartaIAM "github.com/mweagle/Sparta/aws/iam"
	spartaIAMBuilder "github.com/mweagle/Sparta/aws/iam/builder"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/sirupsen/logrus"
)

// Sparta service decorator that creates a Kinesis Firehose stream
// and associates the Lambda transformation function with the incoming
// records
func newFirehoseDecorator(xformer *sparta.LambdaAWSInfo) sparta.ServiceDecoratorHookHandler {
	decorator := func(context map[string]interface{},
		serviceName string,
		template *gocf.Template,
		S3Bucket string,
		S3Key string,
		buildID string,
		awsSession *session.Session,
		noop bool,
		logger *logrus.Logger) error {

		// Get some names...
		deliveryStreamResource := spartaCF.StableResourceName("FirehoseStream")
		iamRoleResource := spartaCF.StableResourceName("FirehoseRole")
		s3BucketResource := spartaCF.StableResourceName("FirehoseBucket")

		// Create the Bucket
		s3Bucket := &gocf.S3Bucket{
			VersioningConfiguration: &gocf.S3BucketVersioningConfiguration{
				Status: gocf.String("Enabled"),
			},
		}
		s3Entry := template.AddResource(s3BucketResource, s3Bucket)
		s3Entry.DeletionPolicy = "Retain"

		// Create the IAM Role
		assumeRoleStatement := spartaIAMBuilder.
			Allow("sts:AssumeRole").
			WithCondition(sparta.ArbitraryJSONObject{
				"StringEquals": sparta.ArbitraryJSONObject{
					"sts:ExternalId": sparta.ArbitraryJSONObject{
						"Ref": "AWS::AccountId",
					},
				},
			},
			).
			ForPrincipals(sparta.KinesisFirehosePrincipal).
			ToPolicyStatement()

		iamRole := &gocf.IAMRole{
			AssumeRolePolicyDocument: sparta.ArbitraryJSONObject{
				"Version": "2012-10-17",
				"Statement": []spartaIAM.PolicyStatement{
					assumeRoleStatement,
				},
			},
			Policies: &gocf.IAMRolePolicyList{
				gocf.IAMRolePolicy{
					PolicyName: gocf.String("FirehoseDeliveryPolicy"),
					PolicyDocument: sparta.ArbitraryJSONObject{
						"Version": "2012-10-17",
						"Statement": []interface{}{
							sparta.ArbitraryJSONObject{
								"Effect": "Allow",
								"Action": []string{"s3:AbortMultipartUpload",
									"s3:GetBucketLocation",
									"s3:GetObject",
									"s3:ListBucket",
									"s3:ListBucketMultipartUploads",
									"s3:PutObject",
								},
								"Resource": []interface{}{
									spartaCF.S3ArnForBucket(gocf.Ref(s3BucketResource)),
									spartaCF.S3AllKeysArnForBucket(gocf.Ref(s3BucketResource)),
								},
							},
							spartaIAMBuilder.Allow("lambda:InvokeFunction",
								"lambda:GetFunctionConfiguration").
								ForResource().Attr(xformer.LogicalResourceName(), "Arn").
								ToPolicyStatement(),
						},
					},
				},
			},
		}
		template.AddResource(iamRoleResource, iamRole)

		// Finally the stream...
		deliveryStream := &gocf.KinesisFirehoseDeliveryStream{
			ExtendedS3DestinationConfiguration: &gocf.KinesisFirehoseDeliveryStreamExtendedS3DestinationConfiguration{
				BucketARN: spartaCF.S3ArnForBucket(gocf.Ref(s3BucketResource)),
				BufferingHints: &gocf.KinesisFirehoseDeliveryStreamBufferingHints{
					IntervalInSeconds: gocf.Integer(60),
					SizeInMBs:         gocf.Integer(50),
				},
				CompressionFormat: gocf.String("UNCOMPRESSED"),
				Prefix:            gocf.String("firehose/"),
				RoleARN:           gocf.GetAtt(iamRoleResource, "Arn"),
				ProcessingConfiguration: &gocf.KinesisFirehoseDeliveryStreamProcessingConfiguration{
					Enabled: gocf.Bool(true),
					Processors: &gocf.KinesisFirehoseDeliveryStreamProcessorList{
						gocf.KinesisFirehoseDeliveryStreamProcessor{
							Type: gocf.String("Lambda"),
							Parameters: &gocf.KinesisFirehoseDeliveryStreamProcessorParameterList{
								gocf.KinesisFirehoseDeliveryStreamProcessorParameter{
									ParameterName:  gocf.String("LambdaArn"),
									ParameterValue: gocf.GetAtt(xformer.LogicalResourceName(), "Arn"),
								},
							},
						},
					},
				},
			},
		}
		deliveryStreamEntry := template.AddResource(deliveryStreamResource, deliveryStream)
		deliveryStreamEntry.DependsOn = []string{iamRoleResource}
		return nil
	}
	return sparta.ServiceDecoratorHookFunc(decorator)
}

////////////////////////////////////////////////////////////////////////////////
// Main
func main() {

	sess := session.Must(session.NewSession())
	awsName, awsNameErr := spartaCF.UserAccountScopedStackName("TransformerStack",
		sess)
	if awsNameErr != nil {
		fmt.Printf("Failed to create stack name: %#v\n", awsNameErr)
		os.Exit(1)
	}

	hooks := &sparta.WorkflowHooks{}
	reactorFunc, reactorFuncErr := archetype.NewKinesisFirehoseTransformer("sample.transform",
		5*time.Minute,
		hooks)
	if reactorFuncErr != nil {
		fmt.Printf("Failed to create stack name: %#v\n", reactorFuncErr)
		os.Exit(1)
	}
	hooks.ServiceDecorators = append(hooks.ServiceDecorators, newFirehoseDecorator(reactorFunc))
	// Sanitize the name so that it doesn't have any spaces
	var lambdaFunctions []*sparta.LambdaAWSInfo
	lambdaFunctions = append(lambdaFunctions, reactorFunc)

	err := sparta.MainEx(awsName,
		"Simple Sparta application that demonstrates Kinesis Firehose functionality",
		lambdaFunctions,
		nil,
		nil,
		hooks,
		false)
	if err != nil {
		os.Exit(1)
	}
}
