package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	log "github.com/sirupsen/logrus"
)

type FunctionUrlConfig struct {
	alias      *string `yaml:"_"` // this is set internally
	InvokeMode *string `yaml:"invokeMode"`
}

// equals compares this function url config, with the other & returns true if the two are the same
func (u *FunctionUrlConfig) equals(other *FunctionUrlConfig) resource.EqualsResult {
	if u == nil && other != nil {
		return resource.LeftZero
	} else if u != nil && other == nil {
		return resource.RightZero
	} else if u != nil && other != nil {
		if util.Coalesce(u.alias, "") != util.Coalesce(other.alias, "") ||
			util.Coalesce(u.InvokeMode, string(types.InvokeModeBuffered)) != util.Coalesce(other.InvokeMode, string(types.InvokeModeBuffered)) {
			return resource.NotEqual
		}
	}
	return resource.Equal
}

// normalize sets defaults
func (u *FunctionUrlConfig) normalize() {
	// if not supplied, set InvokeMode to BUFFERED, else capitalize what ever is supplied
	if u.InvokeMode == nil {
		u.InvokeMode = aws.String(string(types.InvokeModeBuffered))
	} else {
		u.InvokeMode = aws.String(strings.ToUpper(*u.InvokeMode))
	}
}

// validate
func (u *FunctionUrlConfig) validate() []string {
	var errorMsgs []string
	switch *u.InvokeMode {
	case string(types.InvokeModeBuffered), string(types.InvokeModeResponseStream):
		// no-op
	default:
		errorMsgs = append(errorMsgs, fmt.Sprintf("invalid invoke mode supplied %q for lambda function url", *u.InvokeMode))
	}

	if len(errorMsgs) != 0 {
		return errorMsgs
	}

	return nil
}

// apply makes a new function url for the supplied function name
func (u *FunctionUrlConfig) apply(ctx context.Context, rctx resource.Context, functionName string) error {
	client := client.Lambda(ctx, rctx.ProviderName)
	out, err := client.CreateFunctionUrlConfig(ctx, &lambda.CreateFunctionUrlConfigInput{
		FunctionName: aws.String(functionName),
		AuthType:     types.FunctionUrlAuthTypeNone, // forced to none
		InvokeMode:   types.InvokeMode(*u.InvokeMode),
		Qualifier:    u.alias,
	})
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Function Name": functionName,
		"Qualifier":     util.Coalesce(u.alias, "$LATEST"),
		"Function url":  *out.FunctionUrl,
	}).Info(color.Green("lambda function url created"))

	return nil
}

// apply makes a new function url for the supplied function name
func (u *FunctionUrlConfig) update(ctx context.Context, rctx resource.Context, functionName string) error {
	client := client.Lambda(ctx, rctx.ProviderName)
	out, err := client.UpdateFunctionUrlConfig(ctx, &lambda.UpdateFunctionUrlConfigInput{
		FunctionName: aws.String(functionName),
		AuthType:     types.FunctionUrlAuthTypeNone, // forced to none
		InvokeMode:   types.InvokeMode(*u.InvokeMode),
		Qualifier:    u.alias,
	})
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Function Name": functionName,
		"Qualifier":     util.Coalesce(u.alias, "$LATEST"),
		"Function url":  *out.FunctionUrl,
	}).Info(color.Yellow("lambda function url updated"))

	return nil
}

// destroy this lambda function config for the supplied function name
func (u *FunctionUrlConfig) destroy(ctx context.Context, rctx resource.Context, functionName string) error {
	client := client.Lambda(ctx, rctx.ProviderName)
	_, err := client.DeleteFunctionUrlConfig(ctx, &lambda.DeleteFunctionUrlConfigInput{
		FunctionName: aws.String(functionName),
		Qualifier:    u.alias,
	})
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Function Name": functionName,
		"Qualifier":     util.Coalesce(u.alias, "$LATEST"),
	}).Info(color.Red("lambda function url deleted"))

	return nil
}
