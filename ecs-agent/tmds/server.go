// copyright amazon.com inc. or its affiliates. all rights reserved.
//
// licensed under the apache license, version 2.0 (the "license"). you may
// not use this file except in compliance with the license. a copy of the
// license is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. this file is distributed
// on an "as is" basis, without warranties or conditions of any kind, either
// express or implied. see the license for the specific language governing
// permissions and limitations under the license.
package tmds

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/didip/tollbooth"
	"github.com/gorilla/mux"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/request"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/logging"
	muxutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/mux"
)

const (
	// TMDS IP and port
	IPv4 = "127.0.0.1"
	Port = 51679
)

// IPv4 address for TMDS
func AddressIPv4() string {
	return fmt.Sprintf("%s:%d", IPv4, Port)
}

// Configuration for TMDS
type Config struct {
	listenAddress   string        // http server listen address
	readTimeout     time.Duration // http server read timeout
	writeTimeout    time.Duration // http server write timeout
	steadyStateRate float64       // steady request rate limit
	burstRate       int           // burst request rate limit
	router          *mux.Router   // router with routes configured
}

// Function type for updating TMDS config
type ConfigOpt func(*Config) error

// Set TMDS listen address
func WithListenAddress(listenAddr string) ConfigOpt {
	return func(c *Config) error {
		c.listenAddress = listenAddr
		return nil
	}
}

// Set TMDS read timeout
func WithReadTimeout(readTimeout time.Duration) ConfigOpt {
	return func(c *Config) error {
		c.readTimeout = readTimeout
		return nil
	}
}

// Set TMDS write timeout
func WithWriteTimeout(writeTimeout time.Duration) ConfigOpt {
	return func(c *Config) error {
		c.writeTimeout = writeTimeout
		return nil
	}
}

// Set TMDS steady request rate limit
func WithSteadyStateRate(steadyStateRate float64) ConfigOpt {
	return func(c *Config) error {
		c.steadyStateRate = steadyStateRate
		return nil
	}
}

// Set TMDS burst request rate limit
func WithBurstRate(burstRate int) ConfigOpt {
	return func(c *Config) error {
		c.burstRate = burstRate
		return nil
	}
}

// Set TMDS router
func WithRouter(router *mux.Router) ConfigOpt {
	return func(c *Config) error {
		c.router = router
		return nil
	}
}

// Create a new HTTP Task Metadata Server (TMDS)
func NewServer(auditLogger audit.AuditLogger, options ...ConfigOpt) (*http.Server, error) {
	config := new(Config)
	for _, opt := range options {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	return setup(auditLogger, config)
}

func setup(auditLogger audit.AuditLogger, config *Config) (*http.Server, error) {
	if config.listenAddress == "" {
		return nil, errors.New("listenAddress cannot be empty")
	}
	if config.router == nil {
		return nil, errors.New("router cannot be nil")
	}

	// Define a reqeuest rate limiter
	limiter := tollbooth.
		NewLimiter(config.steadyStateRate, nil).
		SetOnLimitReached(limitReachedHandler(auditLogger)).
		SetBurst(config.burstRate)

	// Log all requests and then pass through to muxRouter.
	loggingMuxRouter := mux.NewRouter()

	// rootPath is a path for any traffic to this endpoint
	rootPath := "/" + muxutils.ConstructMuxVar("root", muxutils.AnythingRegEx)
	loggingMuxRouter.Handle(rootPath, tollbooth.LimitHandler(
		limiter, logging.NewLoggingHandler(config.router)))

	// explicitly enable path cleaning
	loggingMuxRouter.SkipClean(false)

	return &http.Server{
		Addr:         config.listenAddress,
		Handler:      loggingMuxRouter,
		ReadTimeout:  config.readTimeout,
		WriteTimeout: config.writeTimeout,
	}, nil
}

// LimitReachedHandler logs the throttled request in the credentials audit log
func limitReachedHandler(auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logRequest := request.LogRequest{
			Request: r,
		}
		auditLogger.Log(logRequest, http.StatusTooManyRequests, "")
	}
}
