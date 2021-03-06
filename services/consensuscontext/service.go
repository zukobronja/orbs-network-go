package consensuscontext

import (
	"context"
	"github.com/orbs-network/orbs-network-go/config"
	"github.com/orbs-network/orbs-network-go/crypto/digest"
	"github.com/orbs-network/orbs-network-go/instrumentation/log"
	"github.com/orbs-network/orbs-network-go/instrumentation/metric"
	"github.com/orbs-network/orbs-network-go/instrumentation/trace"
	"github.com/orbs-network/orbs-spec/types/go/services"
	"time"
)

var LogTag = log.Service("consensus-context")

type metrics struct {
	createTxBlockTime      *metric.Histogram
	createResultsBlockTime *metric.Histogram
	transactionsRate       *metric.Rate
}

func newMetrics(factory metric.Factory) *metrics {
	return &metrics{
		createTxBlockTime:      factory.NewLatency("ConsensusContext.CreateTransactionsBlockTime", 10*time.Second),
		createResultsBlockTime: factory.NewLatency("ConsensusContext.CreateResultsBlockTime", 10*time.Second),
		transactionsRate:       factory.NewRate("ConsensusContext.TransactionsPerSecond"),
	}
}

type service struct {
	transactionPool services.TransactionPool
	virtualMachine  services.VirtualMachine
	stateStorage    services.StateStorage
	config          config.ConsensusContextConfig
	logger          log.BasicLogger

	metrics *metrics
}

func NewConsensusContext(
	transactionPool services.TransactionPool,
	virtualMachine services.VirtualMachine,
	stateStorage services.StateStorage,
	config config.ConsensusContextConfig,
	logger log.BasicLogger,
	metricFactory metric.Factory,
) services.ConsensusContext {

	return &service{
		transactionPool: transactionPool,
		virtualMachine:  virtualMachine,
		stateStorage:    stateStorage,
		config:          config,
		logger:          logger.WithTags(LogTag),
		metrics:         newMetrics(metricFactory),
	}
}

func (s *service) RequestNewTransactionsBlock(ctx context.Context, input *services.RequestNewTransactionsBlockInput) (*services.RequestNewTransactionsBlockOutput, error) {
	logger := s.logger.WithTags(trace.LogFieldFrom(ctx))
	txBlock, err := s.createTransactionsBlock(ctx, input.BlockHeight, input.PrevBlockHash)
	if err != nil {
		return nil, err
	}

	logger.Info("created Transactions block", log.Int("num-transactions", len(txBlock.SignedTransactions)), log.Stringable("transactions-block", txBlock))

	s.metrics.transactionsRate.Measure(int64(len(txBlock.SignedTransactions)))

	for _, tx := range txBlock.SignedTransactions {
		txHash := digest.CalcTxHash(tx.Transaction())
		logger.Info("transaction entered transactions block", log.String("flow", "checkpoint"), log.Transaction(txHash), log.BlockHeight(txBlock.Header.BlockHeight()))
	}

	return &services.RequestNewTransactionsBlockOutput{
		TransactionsBlock: txBlock,
	}, nil
}

func (s *service) RequestNewResultsBlock(ctx context.Context, input *services.RequestNewResultsBlockInput) (*services.RequestNewResultsBlockOutput, error) {
	logger := s.logger.WithTags(trace.LogFieldFrom(ctx))

	rxBlock, err := s.createResultsBlock(ctx, input.BlockHeight, input.PrevBlockHash, input.TransactionsBlock)
	if err != nil {
		return nil, err
	}

	logger.Info("created Results block", log.Stringable("results-block", rxBlock))

	return &services.RequestNewResultsBlockOutput{
		ResultsBlock: rxBlock,
	}, nil
}

func (s *service) ValidateTransactionsBlock(ctx context.Context, input *services.ValidateTransactionsBlockInput) (*services.ValidateTransactionsBlockOutput, error) {
	panic("Not implemented")
}

func (s *service) ValidateResultsBlock(ctx context.Context, input *services.ValidateResultsBlockInput) (*services.ValidateResultsBlockOutput, error) {
	panic("Not implemented")
}
