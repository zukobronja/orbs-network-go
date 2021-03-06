package native

import (
	"context"
	"github.com/orbs-network/orbs-contract-sdk/go/sdk"
	"github.com/orbs-network/orbs-network-go/instrumentation/log"
	"github.com/orbs-network/orbs-network-go/instrumentation/metric"
	"github.com/orbs-network/orbs-network-go/instrumentation/trace"
	"github.com/orbs-network/orbs-network-go/services/processor/native/adapter"
	"github.com/orbs-network/orbs-spec/types/go/protocol"
	"github.com/orbs-network/orbs-spec/types/go/services"
	"github.com/orbs-network/orbs-spec/types/go/services/handlers"
	"sync"
	"time"
)

var LogTag = log.Service("processor-native")

type service struct {
	logger   log.BasicLogger
	compiler adapter.Compiler

	mutex                         *sync.RWMutex
	contractSdkHandlerUnderMutex  handlers.ContractSdkCallHandler
	contractInstancesUnderMutex   map[string]sdk.ContractInstance
	deployableContractsUnderMutex map[string]*sdk.ContractInfo

	metrics *metrics
}

type metrics struct {
	deployedContracts       *metric.Gauge
	processCallTime         *metric.Histogram
	contractCompilationTime *metric.Histogram
}

func getMetrics(m metric.Factory) *metrics {
	return &metrics{
		deployedContracts:       m.NewGauge("Processor.Native.DeployedContractsNumber"),
		processCallTime:         m.NewLatency("Processor.Native.ProcessCallTime", 10*time.Second),
		contractCompilationTime: m.NewLatency("Processor.Native.ContractCompilationTime", 10*time.Second),
	}
}

func NewNativeProcessor(
	compiler adapter.Compiler,
	logger log.BasicLogger,
	metricFactory metric.Factory,
) services.Processor {
	return &service{
		compiler: compiler,
		logger:   logger.WithTags(LogTag),
		mutex:    &sync.RWMutex{},
		metrics:  getMetrics(metricFactory),
	}
}

// runs once on system initialization (called by the virtual machine constructor)
func (s *service) RegisterContractSdkCallHandler(handler handlers.ContractSdkCallHandler) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.contractSdkHandlerUnderMutex = handler

	if s.contractInstancesUnderMutex == nil && s.deployableContractsUnderMutex == nil {
		s.contractInstancesUnderMutex = initializePreBuiltRepositoryContractInstances(handler)
		s.deployableContractsUnderMutex = make(map[string]*sdk.ContractInfo)
	}
}

func (s *service) ProcessCall(ctx context.Context, input *services.ProcessCallInput) (*services.ProcessCallOutput, error) {
	logger := s.logger.WithTags(trace.LogFieldFrom(ctx))
	// retrieve code
	executionContextId := sdk.Context(input.ContextId)
	contractInfo, methodInfo, err := s.retrieveContractAndMethodInfoFromRepository(ctx, executionContextId, string(input.ContractName), string(input.MethodName))
	if err != nil {
		return &services.ProcessCallOutput{
			// TODO: do we need to remove system errors from OutputArguments? https://github.com/orbs-network/orbs-spec/issues/97
			OutputArgumentArray: s.createMethodOutputArgsWithString(err.Error()),
			CallResult:          protocol.EXECUTION_RESULT_ERROR_UNEXPECTED,
		}, err
	}

	// check permissions
	err = s.verifyMethodPermissions(contractInfo, methodInfo, input.CallingService, input.CallingPermissionScope, input.AccessScope)
	if err != nil {
		return &services.ProcessCallOutput{
			// TODO: do we need to remove system errors from OutputArguments? https://github.com/orbs-network/orbs-spec/issues/97
			OutputArgumentArray: s.createMethodOutputArgsWithString(err.Error()),
			CallResult:          protocol.EXECUTION_RESULT_ERROR_UNEXPECTED,
		}, err
	}

	start := time.Now()
	defer s.metrics.processCallTime.RecordSince(start)

	// execute
	logger.Info("processor executing contract", log.String("contract", contractInfo.Name), log.String("method", methodInfo.Name))

	outputArgs, contractErr, err := s.processMethodCall(executionContextId, contractInfo, methodInfo, input.InputArgumentArray)
	if outputArgs == nil {
		outputArgs = (&protocol.MethodArgumentArrayBuilder{}).Build()
	}
	if err != nil {
		logger.Info("contract execution failed", log.Error(err))

		return &services.ProcessCallOutput{
			// TODO: do we need to remove system errors from OutputArguments? https://github.com/orbs-network/orbs-spec/issues/97
			OutputArgumentArray: s.createMethodOutputArgsWithString(err.Error()),
			CallResult:          protocol.EXECUTION_RESULT_ERROR_UNEXPECTED,
		}, err
	}

	// result
	callResult := protocol.EXECUTION_RESULT_SUCCESS
	if contractErr != nil {
		logger.Info("contract returned error", log.Error(contractErr))

		callResult = protocol.EXECUTION_RESULT_ERROR_SMART_CONTRACT
	}
	return &services.ProcessCallOutput{
		OutputArgumentArray: outputArgs,
		CallResult:          callResult,
	}, contractErr
}

func (s *service) GetContractInfo(ctx context.Context, input *services.GetContractInfoInput) (*services.GetContractInfoOutput, error) {
	// retrieve code
	executionContextId := sdk.Context(input.ContextId)
	contractInfo, err := s.retrieveContractInfoFromRepository(ctx, executionContextId, string(input.ContractName))
	if err != nil {
		return nil, err
	}

	// result
	return &services.GetContractInfoOutput{
		PermissionScope: protocol.ExecutionPermissionScope(contractInfo.Permission),
	}, nil
}

func (s *service) getContractSdkHandler() handlers.ContractSdkCallHandler {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.contractSdkHandlerUnderMutex
}

func (s *service) getContractInstanceFromRepository(contractName string) sdk.ContractInstance {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.contractInstancesUnderMutex == nil {
		return nil
	}
	return s.contractInstancesUnderMutex[contractName]
}

func (s *service) addContractInstanceToRepository(contractName string, contractInstance sdk.ContractInstance) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.contractInstancesUnderMutex == nil {
		return
	}
	s.contractInstancesUnderMutex[contractName] = contractInstance
}

func (s *service) getDeployableContractInfoFromRepository(contractName string) *sdk.ContractInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.deployableContractsUnderMutex == nil {
		return nil
	}
	return s.deployableContractsUnderMutex[contractName]
}

func (s *service) addDeployableContractInfoToRepository(contractName string, contractInfo *sdk.ContractInfo) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.deployableContractsUnderMutex == nil {
		return
	}
	s.deployableContractsUnderMutex[contractName] = contractInfo
}
