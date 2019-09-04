package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"time"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/route53"
)
import "github.com/aws/aws-sdk-go/aws/session"

func certificateResource(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, e error) {
	physicalResourceID = "foo"
	if event.ResourceProperties["Domain"] == nil {
		e = errors.New("'Domain' property is required")
		return
	}
	domain := event.ResourceProperties["Domain"].(string)
	if event.ResourceProperties["HostedZone"] == nil {
		e = errors.New("'HostedZone' property is required")
		return
	}
	hostedZone := event.ResourceProperties["HostedZone"].(string)

	if event.RequestType == cfn.RequestCreate {
		fmt.Println("Received CREATE event.")

		session, err := NewSession()
		if err != nil {
			e = err
			return
		}
		acmService := acm.New(session)
		certificateRequestOutput, err := acmService.RequestCertificate(&acm.RequestCertificateInput{
			DomainName:       aws.String(domain),
			ValidationMethod: aws.String("DNS"),
		})
		if err != nil {
			e = err
			return
		}
		physicalResourceID = *certificateRequestOutput.CertificateArn
		fmt.Println("Created certificate request.")
		time.Sleep(15 * time.Second)
		certificate, err := acmService.DescribeCertificate(&acm.DescribeCertificateInput{CertificateArn: certificateRequestOutput.CertificateArn})
		if err != nil {
			e = err
			return
		}
		recordName := certificate.Certificate.DomainValidationOptions[0].ResourceRecord.Name
		recordValue := certificate.Certificate.DomainValidationOptions[0].ResourceRecord.Value

		route53Service := route53.New(session)

		hostedZoneOutput, err := route53Service.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: aws.String(hostedZone + ".")})
		if err != nil {
			e = err
			return
		}

		_, err = route53Service.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			HostedZoneId: hostedZoneOutput.HostedZones[0].Id,
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{
					{
						Action: aws.String("CREATE"),
						ResourceRecordSet: &route53.ResourceRecordSet{
							Name: recordName,
							Type: aws.String("CNAME"),
							ResourceRecords: []*route53.ResourceRecord{
								{Value: recordValue},
							},
							TTL: aws.Int64(60),
						},
					},
				},
			},
		})
		if err != nil {
			e = err
			return
		}
	} else if event.RequestType == cfn.RequestDelete {
		fmt.Println("Received DELETE event.")

		session, err := NewSession()
		if err != nil {
			e = err
			return
		}
		acmService := acm.New(session)
		certificates, err := acmService.ListCertificates(&acm.ListCertificatesInput{})
		if err != nil {
			e = err
			return
		}
		for _, cert := range certificates.CertificateSummaryList {
			if *cert.DomainName == domain {
				certificate, err := acmService.DescribeCertificate(&acm.DescribeCertificateInput{CertificateArn: cert.CertificateArn})
				if err != nil {
					e = err
					return
				}

				recordName := certificate.Certificate.DomainValidationOptions[0].ResourceRecord.Name
				recordValue := certificate.Certificate.DomainValidationOptions[0].ResourceRecord.Value

				route53Service := route53.New(session)

				hostedZoneOutput, err := route53Service.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: aws.String(hostedZone + ".")})
				if err != nil {
					e = err
					return
				}

				_, err = route53Service.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
					HostedZoneId: hostedZoneOutput.HostedZones[0].Id,
					ChangeBatch: &route53.ChangeBatch{
						Changes: []*route53.Change{
							{
								Action: aws.String("DELETE"),
								ResourceRecordSet: &route53.ResourceRecordSet{
									Name: recordName,
									Type: aws.String("CNAME"),
									ResourceRecords: []*route53.ResourceRecord{
										{Value: recordValue},
									},
									TTL: aws.Int64(60),
								},
							},
						},
					},
				})
				if err != nil {
					e = err
					return
				}

				_, err = acmService.DeleteCertificate(&acm.DeleteCertificateInput{CertificateArn: cert.CertificateArn})
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
	} else if event.RequestType == cfn.RequestUpdate {
		fmt.Println("Received UPDATE event. Ignoring.")
	}

	return
}

func main() {
	lambda.Start(cfn.LambdaWrap(certificateResource))
}

// NewSession returns session which respects:
// - environment variables
// - `~/aws/.config` and `~/aws/.credentials` files
//
// Example:
//
//     import "github.com/hekonsek/awsom-session"
//     ...
//     err, sess := awsom_session.NewSession()
func NewSession() (*session.Session, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	return sess, nil
}