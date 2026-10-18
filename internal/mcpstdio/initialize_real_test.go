package mcpstdio

// THE FINDING'S OWN CRITERION, ON THE SHIPPED PATH (nocx-xn63t.6.45).
//
// The finding says the initialize exchange must carry role-appropriate
// instructions, and that a coordinator must be sent to its nocx tools rather
// than to another multiplexer. A test that handed the adapter a canned answer
// would prove the adapter copies a string; this one starts the SHIPPED endpoint
// over a real socket, with the grant an authorization mints for each role, and
// speaks MCP initialize to the SHIPPED adapter. What comes back is what a
// coordinator's client would read.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agenttools"
	"github.com/shady2k/nocx/internal/assistant"
	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/toolendpoint"
)

// grantAuthorizer admits the connection with the authority the test is about,
// which is the only thing that decides what the endpoint tells the session.
type grantAuthorizer struct{ grant content.Grant }

func (a grantAuthorizer) Admit(_ toolendpoint.Peer, publish func(string, toolendpoint.AdmissionEpoch) bool) (assistant.ToolInvocation, func(), error) {
	if !publish("session", 1) {
		return assistant.ToolInvocation{}, nil, toolendpoint.ErrNotEnrolled
	}
	return assistant.ToolInvocation{
		Context:    context.Background(),
		RunContext: agenttools.RunContext{Session: "session"},
		Grant:      a.grant,
	}, func() {}, nil
}

func TestInitializeTellsEachRoleWhatItMayDoOverTheRealEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name string
		// grant is the authority the admitted connection carries.
		grant content.Grant
		// want is the call this role's instructions must name, and absent is
		// the call they must not: a worker told to spawn is told to make a call
		// its grant refuses.
		want   string
		absent string
	}{
		{
			name: "coordinator",
			grant: content.Grant{
				Effects: []content.Effect{
					content.EffectObserve,
					content.EffectMutateReversible,
					content.EffectMutateDestructive,
					content.EffectDelegate,
				},
				Scopes: []content.GrantScope{{Kind: content.ResourceSession, ID: "session"}},
			},
			want: "workers.spawn",
		},
		{
			name:  "worker",
			grant: content.Grant{Effects: []content.Effect{content.EffectObserve}},
			want:  "workers.report",
			// A worker's grant permits nothing destructive either, so the
			// instructions must not send it to the call that ends a worker.
			absent: "workers.close",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := startRealEndpoint(t, grantAuthorizer{grant: tc.grant}, &heldDispatcher{
				held: make(chan struct{}), release: make(chan struct{}),
			})
			driver := startStdio(t, endpoint.SocketPath())

			driver.send(0, "initialize", `{"protocolVersion":"2025-11-25"}`)
			envelope := driver.nextID(0)
			var initialized struct {
				ServerInfo   map[string]string `json:"serverInfo"`
				Instructions string            `json:"instructions"`
			}
			if err := json.Unmarshal(envelope["result"], &initialized); err != nil {
				t.Fatalf("decode initialize result %v: %v", envelope, err)
			}
			if initialized.ServerInfo["name"] == "" {
				t.Fatalf("initialize carried no serverInfo: %v", envelope)
			}
			if !strings.Contains(initialized.Instructions, tc.want) {
				t.Fatalf("a %s's initialize instructions do not name %s: %q", tc.name, tc.want, initialized.Instructions)
			}
			if tc.absent != "" && strings.Contains(initialized.Instructions, tc.absent) {
				t.Fatalf("a %s's initialize instructions name %s, which its grant does not offer: %q", tc.name, tc.absent, initialized.Instructions)
			}
		})
	}
}
