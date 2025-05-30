// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package e2e

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
	dtlsnet "github.com/pion/dtls/v3/pkg/net"
	transportTest "github.com/pion/transport/v3/test"
)

const (
	flightInterval   = time.Millisecond * 100
	lossyTestTimeout = 30 * time.Second
)

// DTLS Client/Server over a lossy transport, just asserts it can handle at increasing increments

func TestPionE2ELossy(t *testing.T) { //nolint:cyclop
	// Check for leaking routines
	report := transportTest.CheckRoutines(t)
	defer report()

	type runResult struct {
		dtlsConn *dtls.Conn
		err      error
	}

	serverCert, err := selfsign.GenerateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	clientCert, err := selfsign.GenerateSelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		LossChanceRange             int
		DoClientAuth                bool
		CipherSuites                []dtls.CipherSuiteID
		MTU                         int
		DisableServerFlightInterval bool
	}{
		{
			LossChanceRange: 0,
		},
		{
			LossChanceRange: 10,
		},
		{
			LossChanceRange: 20,
		},
		{
			LossChanceRange: 50,
		},
		{
			LossChanceRange: 0,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 10,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 20,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 50,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 0,
			CipherSuites:    []dtls.CipherSuiteID{dtls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA},
		},
		{
			LossChanceRange: 10,
			CipherSuites:    []dtls.CipherSuiteID{dtls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA},
		},
		{
			LossChanceRange: 20,
			CipherSuites:    []dtls.CipherSuiteID{dtls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA},
		},
		{
			LossChanceRange: 50,
			CipherSuites:    []dtls.CipherSuiteID{dtls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA},
		},
		{
			LossChanceRange: 10,
			MTU:             100,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 20,
			MTU:             100,
			DoClientAuth:    true,
		},
		{
			LossChanceRange: 50,
			MTU:             100,
			DoClientAuth:    true,
		},
		// Incoming retransmitted handshakes should cause us to retransmit. Disabling the FlightInterval on one side
		// means that a incoming re-transmissions causes the retransmission to be fired
		{
			LossChanceRange:             10,
			DisableServerFlightInterval: true,
		},
		{
			LossChanceRange:             20,
			DisableServerFlightInterval: true,
		},
		{
			LossChanceRange:             50,
			DisableServerFlightInterval: true,
		},
	} {
		name := fmt.Sprintf("Loss%d_MTU%d", test.LossChanceRange, test.MTU)
		if test.DoClientAuth {
			name += "_WithCliAuth"
		}
		for _, ciph := range test.CipherSuites {
			name += "_With" + ciph.String()
		}
		if test.DisableServerFlightInterval {
			name += "_WithNoServerFlightInterval"
		}

		test := test
		t.Run(name, func(t *testing.T) {
			// Limit runtime in case of deadlocks
			lim := transportTest.TimeOut(lossyTestTimeout + time.Second)
			defer lim.Stop()

			rand.Seed(time.Now().UTC().UnixNano())
			chosenLoss := rand.Intn(9) + test.LossChanceRange //nolint:gosec
			serverDone := make(chan runResult)
			clientDone := make(chan runResult)
			br := transportTest.NewBridge()

			if err = br.SetLossChance(chosenLoss); err != nil {
				t.Fatal(err)
			}

			go func() {
				cfg := &dtls.Config{
					FlightInterval:           flightInterval,
					CipherSuites:             test.CipherSuites,
					InsecureSkipVerify:       true,
					MTU:                      test.MTU,
					DisableRetransmitBackoff: true,
				}

				if test.DoClientAuth {
					cfg.Certificates = []tls.Certificate{clientCert}
				}

				client, startupErr := dtls.Client(dtlsnet.PacketConnFromConn(br.GetConn0()), br.GetConn0().RemoteAddr(), cfg)
				clientDone <- runResult{client, startupErr}
			}()

			go func() {
				cfg := &dtls.Config{
					Certificates:             []tls.Certificate{serverCert},
					FlightInterval:           flightInterval,
					MTU:                      test.MTU,
					DisableRetransmitBackoff: true,
				}

				if test.DoClientAuth {
					cfg.ClientAuth = dtls.RequireAnyClientCert
				}

				if test.DisableServerFlightInterval {
					cfg.FlightInterval = time.Hour
				}

				server, startupErr := dtls.Server(dtlsnet.PacketConnFromConn(br.GetConn1()), br.GetConn1().RemoteAddr(), cfg)
				serverDone <- runResult{server, startupErr}
			}()

			testTimer := time.NewTimer(lossyTestTimeout)
			var serverConn, clientConn *dtls.Conn
			defer func() {
				if serverConn != nil {
					if err = serverConn.Close(); err != nil {
						t.Error(err)
					}
				}
				if clientConn != nil {
					if err = clientConn.Close(); err != nil {
						t.Error(err)
					}
				}
			}()

			for {
				if serverConn != nil && clientConn != nil {
					break
				}

				br.Tick()
				select {
				case serverResult := <-serverDone:
					if serverResult.err != nil {
						t.Errorf(
							"Fail, serverError: clientComplete(%t) serverComplete(%t) LossChance(%d) error(%v)",
							clientConn != nil, serverConn != nil, chosenLoss, serverResult.err,
						)

						return
					}

					serverConn = serverResult.dtlsConn
				case clientResult := <-clientDone:
					if clientResult.err != nil {
						t.Errorf(
							"Fail, clientError: clientComplete(%t) serverComplete(%t) LossChance(%d) error(%v)",
							clientConn != nil, serverConn != nil, chosenLoss, clientResult.err,
						)

						return
					}

					clientConn = clientResult.dtlsConn
				case <-testTimer.C:
					t.Errorf(
						"Test expired: clientComplete(%t) serverComplete(%t) LossChance(%d)",
						clientConn != nil, serverConn != nil, chosenLoss,
					)

					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
	}
}
