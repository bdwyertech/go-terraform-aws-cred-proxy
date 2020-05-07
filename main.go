// Encoding: UTF-8
//
// Terraform AWS Credential Proxy Service - Gitlab
//
// Copyright © 2020 Brian Dwyer - Intelligent Digital Services
//

package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gorilla/mux"
)

var disableSharedConfig bool

func init() {
	if flag.Lookup("disable-shared-config") == nil {
		flag.BoolVar(&disableSharedConfig, "disable-shared-config", false, "Disable Shared Configuration (force use of EC2/ECS metadata, ignore AWS_PROFILE, etc.)")
	}
}

func main() {
	// Parse Flags
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	// Handler to work around Hashicorp aws-sdk-go-base issues...
	// https://github.com/hashicorp/aws-sdk-go-base/pull/20
	// export AWS_CRED_CONTAINER_RELATIVE_URI=$AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
	if containerUri := os.Getenv("AWS_CRED_CONTAINER_RELATIVE_URI"); containerUri != "" {
		os.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", containerUri)
	}

	r := mux.NewRouter()
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(getCredentials())
	}).Methods("GET")

	log.Info("Listening on 0.0.0.0:2345")

	srv := &http.Server{
		Addr:    "0.0.0.0:2345",
		Handler: r,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 30,
		IdleTimeout:  time.Second * 60,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// Accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)
	// Block until we receive our signal.
	<-c
	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	log.Info("AWS Credential Proxy - shutting down")
	os.Exit(0)
}

type AwsEc2MetadataCredential struct {
	Code            string `json:"Code"`
	LastUpdated     string `json:"LastUpdated"`
	Type            string `json:"Type"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
}

func getCredentials() AwsEc2MetadataCredential {
	// AWS Session
	sess_opts := session.Options{
		// Config:            *aws.NewConfig().WithRegion("us-east-1"),
		Config:            *aws.NewConfig().WithCredentialsChainVerboseErrors(true),
		SharedConfigState: session.SharedConfigEnable,
	}

	if disableSharedConfig {
		sess_opts.SharedConfigState = session.SharedConfigDisable
	}

	sess := session.Must(session.NewSessionWithOptions(sess_opts))

	creds, err := sess.Config.Credentials.Get()
	if err != nil {
		log.Fatal(err)
	}

	expiresAt, err := sess.Config.Credentials.ExpiresAt()
	if err != nil {
		expiresAt = time.Now().Add(time.Minute * 5)
	}

	return AwsEc2MetadataCredential{
		Code:            "Success",
		LastUpdated:     time.Now().Format(time.RFC3339),
		Type:            "AWS-HMAC",
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		Token:           creds.SessionToken,
		Expiration:      expiresAt.Format(time.RFC3339),
	}
}
