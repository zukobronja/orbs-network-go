package httpserver

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/orbs-network/membuffers/go"
	"github.com/orbs-network/orbs-network-go/instrumentation/log"
	"github.com/orbs-network/orbs-spec/types/go/protocol"
	"github.com/orbs-network/orbs-spec/types/go/protocol/client"
	"github.com/orbs-network/orbs-spec/types/go/services"
)

var LogTag = log.String("adapter", "http-server")

type httpErr struct {
	code     int
	logField *log.Field
	message  string
}

type HttpServer interface {
	GracefulShutdown(timeout time.Duration)
}

type server struct {
	httpServer *http.Server
	logger     log.BasicLogger
	publicApi  services.PublicApi
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func NewHttpServer(address string, logger log.BasicLogger, publicApi services.PublicApi) HttpServer {
	server := &server{
		logger:    logger.WithTags(LogTag),
		publicApi: publicApi,
	}

	if listener, err := server.listen(address); err != nil {
		logger.Error("failed to start http server", log.Error(err))
		panic("failed to start http server")
	} else {
		server.httpServer = &http.Server{
			Addr:    address,
			Handler: server.createRouter(),
		}

		// We prefer not to use `HttpServer.ListenAndServe` because we want to block until the socket is listening or exit immediately
		go server.httpServer.Serve(tcpKeepAliveListener{listener.(*net.TCPListener)})
	}

	logger.Info("started http server", log.String("address", address))

	return server
}

func (s *server) listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func (s *server) GracefulShutdown(timeout time.Duration) {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("failed to stop http server gracefully", log.Error(err))
	}
}

func (s *server) createRouter() http.Handler {
	router := http.NewServeMux()
	router.Handle("/api/v1/send-transaction", report(s.logger, http.HandlerFunc(s.sendTransactionHandler)))
	router.Handle("/api/v1/call-method", report(s.logger, http.HandlerFunc(s.callMethodHandler)))
	router.Handle("/api/v1/get-transaction-status", report(s.logger, http.HandlerFunc(s.getTransactionStatusHandler)))
	return router
}

func report(reporting log.BasicLogger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meter := reporting.Meter("request-process-time", log.String("url", r.URL.String()))
		defer meter.Done()
		h.ServeHTTP(w, r)
	})
}

func (s *server) sendTransactionHandler(w http.ResponseWriter, r *http.Request) {
	bytes, e := readInput(r)
	if e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	clientRequest := client.SendTransactionRequestReader(bytes)
	if e := validate(clientRequest); e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	s.logger.Info("http server received send-transaction", log.Stringable("request", clientRequest))
	result, err := s.publicApi.SendTransaction(&services.SendTransactionInput{ClientRequest: clientRequest})
	if result != nil && result.ClientResponse != nil {
		writeMembuffResponse(w, result.ClientResponse, translateStatusToHttpCode(result.ClientResponse.RequestStatus()), result.ClientResponse.StringTransactionStatus())
	} else {
		writeErrorResponseAndLog(s.logger, w, &httpErr{http.StatusInternalServerError, log.Error(err), err.Error()})
	}
}

func (s *server) callMethodHandler(w http.ResponseWriter, r *http.Request) {
	bytes, e := readInput(r)
	if e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	clientRequest := client.CallMethodRequestReader(bytes)
	if e := validate(clientRequest); e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	s.logger.Info("http server received call-method", log.Stringable("request", clientRequest))
	result, err := s.publicApi.CallMethod(&services.CallMethodInput{ClientRequest: clientRequest})
	if result != nil && result.ClientResponse != nil {
		writeMembuffResponse(w, result.ClientResponse, translateStatusToHttpCode(result.ClientResponse.RequestStatus()), result.ClientResponse.StringCallMethodResult())
	} else {
		writeErrorResponseAndLog(s.logger, w, &httpErr{http.StatusInternalServerError, log.Error(err), err.Error()})
	}
}

func (s *server) getTransactionStatusHandler(w http.ResponseWriter, r *http.Request) {
	bytes, e := readInput(r)
	if e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	clientRequest := client.GetTransactionStatusRequestReader(bytes)
	if e := validate(clientRequest); e != nil {
		writeErrorResponseAndLog(s.logger, w, e)
		return
	}

	s.logger.Info("http server received get-transaction-status", log.Stringable("request", clientRequest))
	result, err := s.publicApi.GetTransactionStatus(&services.GetTransactionStatusInput{ClientRequest: clientRequest})
	if result != nil && result.ClientResponse != nil {
		writeMembuffResponse(w, result.ClientResponse, translateStatusToHttpCode(result.ClientResponse.RequestStatus()), result.ClientResponse.StringTransactionStatus())
	} else {
		writeErrorResponseAndLog(s.logger, w, &httpErr{http.StatusInternalServerError, log.Error(err), err.Error()})
	}
}

func readInput(r *http.Request) ([]byte, *httpErr) {
	if r.Body == nil {
		return nil, &httpErr{http.StatusBadRequest, nil, "http request body is empty"}
	}

	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, &httpErr{http.StatusBadRequest, log.Error(err), "http request body is empty"}
	}
	return bytes, nil
}

func validate(m membuffers.Message) *httpErr {
	if !m.IsValid() {
		return &httpErr{http.StatusBadRequest, log.Stringable("request", m), "http request is not a valid membuffer"}
	}
	return nil
}

func translateStatusToHttpCode(responseCode protocol.RequestStatus) int {
	switch responseCode {
	case protocol.REQUEST_STATUS_COMPLETED:
		return http.StatusOK
	case protocol.REQUEST_STATUS_IN_PROCESS:
		return http.StatusAccepted
	case protocol.REQUEST_STATUS_NOT_FOUND:
		return http.StatusNotFound
	case protocol.REQUEST_STATUS_REJECTED:
		return http.StatusBadRequest
	case protocol.REQUEST_STATUS_CONGESTION:
		return http.StatusServiceUnavailable
	case protocol.REQUEST_STATUS_RESERVED:
		return http.StatusInternalServerError
	}
	return http.StatusNotImplemented
}

func writeMembuffResponse(w http.ResponseWriter, message membuffers.Message, httpCode int, orbsText string) {
	w.Header().Set("Content-Type", "application/vnd.membuffers")
	w.WriteHeader(httpCode)
	w.Header().Set("X-ORBS-CODE-NAME", orbsText)
	w.Write(message.Raw())
}

func writeErrorResponseAndLog(reporting log.BasicLogger, w http.ResponseWriter, m *httpErr) {
	if m.logField == nil {
		reporting.Info(m.message)
	} else {
		reporting.Info(m.message, m.logField)
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(m.code)
	w.Write([]byte(m.message))
}
