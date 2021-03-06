package native

import (
	"context"
	"github.com/orbs-network/orbs-contract-sdk/go/sdk"
	"github.com/orbs-network/orbs-spec/types/go/primitives"
	"github.com/orbs-network/orbs-spec/types/go/protocol"
	"github.com/orbs-network/orbs-spec/types/go/services/handlers"
	"github.com/pkg/errors"
)

type serviceSdk struct {
	handler         handlers.ContractSdkCallHandler
	permissionScope protocol.ExecutionPermissionScope
}

const SDK_OPERATION_NAME_SERVICE = "Sdk.Service"

func (s *serviceSdk) CallMethod(executionContextId sdk.Context, serviceName string, methodName string, args ...interface{}) ([]interface{}, error) {
	output, err := s.handler.HandleSdkCall(context.TODO(), &handlers.HandleSdkCallInput{
		ContextId:     primitives.ExecutionContextId(executionContextId),
		OperationName: SDK_OPERATION_NAME_SERVICE,
		MethodName:    "callMethod",
		InputArguments: []*protocol.MethodArgument{
			(&protocol.MethodArgumentBuilder{
				Name:        "serviceName",
				Type:        protocol.METHOD_ARGUMENT_TYPE_STRING_VALUE,
				StringValue: serviceName,
			}).Build(),
			(&protocol.MethodArgumentBuilder{
				Name:        "methodName",
				Type:        protocol.METHOD_ARGUMENT_TYPE_STRING_VALUE,
				StringValue: methodName,
			}).Build(),
			(&protocol.MethodArgumentBuilder{
				Name:       "inputArgs",
				Type:       protocol.METHOD_ARGUMENT_TYPE_BYTES_VALUE,
				BytesValue: argsToMethodArgumentArray(args...).Raw(),
			}).Build(),
		},
		PermissionScope: s.permissionScope,
	})
	if err != nil {
		return nil, err
	}
	if len(output.OutputArguments) != 1 || !output.OutputArguments[0].IsTypeBytesValue() {
		return nil, errors.Errorf("callMethod Sdk.Service returned corrupt output value")
	}
	methodArgumentArray := protocol.MethodArgumentArrayReader(output.OutputArguments[0].BytesValue())
	return methodArgumentArrayToArgs(methodArgumentArray), nil
}

func argsToMethodArgumentArray(args ...interface{}) *protocol.MethodArgumentArray {
	res := []*protocol.MethodArgumentBuilder{}
	for _, arg := range args {
		switch arg.(type) {
		case uint32:
			res = append(res, &protocol.MethodArgumentBuilder{Name: "uint32", Type: protocol.METHOD_ARGUMENT_TYPE_UINT_32_VALUE, Uint32Value: arg.(uint32)})
		case uint64:
			res = append(res, &protocol.MethodArgumentBuilder{Name: "uint64", Type: protocol.METHOD_ARGUMENT_TYPE_UINT_64_VALUE, Uint64Value: arg.(uint64)})
		case string:
			res = append(res, &protocol.MethodArgumentBuilder{Name: "string", Type: protocol.METHOD_ARGUMENT_TYPE_STRING_VALUE, StringValue: arg.(string)})
		case []byte:
			res = append(res, &protocol.MethodArgumentBuilder{Name: "bytes", Type: protocol.METHOD_ARGUMENT_TYPE_BYTES_VALUE, BytesValue: arg.([]byte)})
		}
	}
	return (&protocol.MethodArgumentArrayBuilder{Arguments: res}).Build()
}

func methodArgumentArrayToArgs(methodArgumentArray *protocol.MethodArgumentArray) []interface{} {
	res := []interface{}{}
	for i := methodArgumentArray.ArgumentsIterator(); i.HasNext(); {
		methodArgument := i.NextArguments()
		switch methodArgument.Type() {
		case protocol.METHOD_ARGUMENT_TYPE_UINT_32_VALUE:
			res = append(res, methodArgument.Uint32Value())
		case protocol.METHOD_ARGUMENT_TYPE_UINT_64_VALUE:
			res = append(res, methodArgument.Uint64Value())
		case protocol.METHOD_ARGUMENT_TYPE_STRING_VALUE:
			res = append(res, methodArgument.StringValue())
		case protocol.METHOD_ARGUMENT_TYPE_BYTES_VALUE:
			res = append(res, methodArgument.BytesValue())
		}
	}
	return res
}
