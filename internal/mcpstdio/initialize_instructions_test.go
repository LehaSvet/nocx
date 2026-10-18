package mcpstdio

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/shady2k/nocx/internal/storage/storagetest"
)

// ── the initialize instructions (nocx-xn63t.6.45) ───────────────────────────

// instructionText is what a fake endpoint says when the adapter asks what this
// session should be told. It is deliberately NOT the product's own text: the
// adapter's job is to carry the answer through, and an assertion written
// against a copy of the real sentence would pass while the adapter invented
// its own.
const instructionText = "these are the endpoint's own instructions"

// answerInstructions answers tools.instructions when that is what the request
// is, and reports whether it did. Every fake endpoint in this package calls it
// first: initialize asks this question, and a fake that ignored it would leave
// initialize waiting on its own bound in every test that initializes.
func answerInstructions(t *testing.T, conn net.Conn, request rpcEnvelope) bool {
	t.Helper()
	if request.Method != "tools.instructions" {
		return false
	}
	writeJSONLine(t, conn, rpcEnvelope{
		JSONRPC: "2.0", ID: request.ID,
		Result: json.RawMessage(`{"instructions":"` + instructionText + `"}`),
	})
	return true
}

// THE SENTENCE THAT WAS MISSING. A coordinator's client initialized this
// server and learned only what it was called and which version it was; the
// finding is that nothing told it to use the tools it had. The answer carries
// the endpoint's instructions verbatim, beside serverInfo, in the initialize
// result the MCP protocol puts them in.
func TestInitializeCarriesTheInstructionsBesideServerInfo(t *testing.T) {
	var asked []string
	socket := startEndpoint(t, func(conn net.Conn, request rpcEnvelope) {
		asked = append(asked, request.Method)
		if answerInstructions(t, conn, request) {
			return
		}
		writeJSONLine(t, conn, rpcEnvelope{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{"tools":[]}`)})
	})

	lines := splitJSONLines(t, runAdapter(t, socket,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`+"\n"))
	if len(lines) != 1 {
		t.Fatalf("responses = %d, want 1", len(lines))
	}
	var initialized struct {
		Result struct {
			ProtocolVersion string            `json:"protocolVersion"`
			ServerInfo      map[string]string `json:"serverInfo"`
			Instructions    *string           `json:"instructions"`
		} `json:"result"`
	}
	mustDecode(t, lines[0], &initialized)
	if initialized.Result.ServerInfo["name"] == "" {
		t.Fatalf("initialize result carries no serverInfo: %s", lines[0])
	}
	if initialized.Result.Instructions == nil || *initialized.Result.Instructions != instructionText {
		t.Fatalf("initialize instructions = %v, want the endpoint's own text %q: %s",
			initialized.Result.Instructions, instructionText, lines[0])
	}
	if len(asked) != 1 || asked[0] != "tools.instructions" {
		t.Fatalf("initialize asked the endpoint for %v, want exactly tools.instructions", asked)
	}
}

// A DEGRADED ANSWER IS STILL AN ANSWER. The endpoint declining to answer the
// instructions question must not fail the handshake: a client whose initialize
// fails has no session at all, and this process is the only way it reaches the
// tools. So the result carries serverInfo and no instructions field — absent,
// not an empty string, which a client would render as an empty instruction.
func TestInitializeAnswersWithoutInstructionsWhenTheEndpointDeclines(t *testing.T) {
	socket := startEndpoint(t, func(conn net.Conn, request rpcEnvelope) {
		writeJSONLine(t, conn, rpcEnvelope{
			JSONRPC: "2.0", ID: request.ID,
			Error: &rpcErrorEnvelope{Code: -32000, Message: "refused", Data: map[string]string{"reason": "no instructions for this caller"}},
		})
	})

	lines := splitJSONLines(t, runAdapter(t, socket,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`+"\n"))
	if len(lines) != 1 {
		t.Fatalf("responses = %d, want 1", len(lines))
	}
	var initialized struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  any                        `json:"error"`
	}
	mustDecode(t, lines[0], &initialized)
	if initialized.Error != nil {
		t.Fatalf("initialize failed because the endpoint declined the instructions: %s", lines[0])
	}
	if _, ok := initialized.Result["serverInfo"]; !ok {
		t.Fatalf("initialize result lost serverInfo: %s", lines[0])
	}
	if raw, ok := initialized.Result["instructions"]; ok {
		t.Fatalf("initialize sent instructions the endpoint never gave (%s): %s", raw, lines[0])
	}
}

// And the endpoint being GONE is the same case: the bridge came up, the
// handshake is answered, and only the sentence is missing.
func TestInitializeAnswersWhenTheEndpointIsGone(t *testing.T) {
	socket := filepath.Join(storagetest.SocketDir(t), "missing.sock")
	lines := splitJSONLines(t, runAdapter(t, socket,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`+"\n"))
	if len(lines) != 1 {
		t.Fatalf("responses = %d, want 1", len(lines))
	}
	var initialized struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  any                        `json:"error"`
	}
	mustDecode(t, lines[0], &initialized)
	if initialized.Error != nil {
		t.Fatalf("initialize failed with no endpoint: %s", lines[0])
	}
	if _, ok := initialized.Result["instructions"]; ok {
		t.Fatalf("initialize sent instructions with no endpoint to ask: %s", lines[0])
	}
}

// AN EMPTY ANSWER IS NOT AN INSTRUCTION. A result with no text in it must leave
// the field out, not send "": a client renders what it is given, and an empty
// string reads as an instruction somebody wrote and then emptied.
func TestInitializeOmitsAnEmptyInstruction(t *testing.T) {
	socket := startEndpoint(t, func(conn net.Conn, request rpcEnvelope) {
		if request.Method == "tools.instructions" {
			writeJSONLine(t, conn, rpcEnvelope{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{"instructions":"   "}`)})
			return
		}
		writeJSONLine(t, conn, rpcEnvelope{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{"tools":[]}`)})
	})
	lines := splitJSONLines(t, runAdapter(t, socket,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`+"\n"))
	var initialized struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	mustDecode(t, lines[0], &initialized)
	if raw, ok := initialized.Result["instructions"]; ok {
		t.Fatalf("initialize sent a blank instruction (%s) instead of leaving it out: %s", raw, lines[0])
	}
}
