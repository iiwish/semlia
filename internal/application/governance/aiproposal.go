package governance

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The AI structured-output gate (SSOT §8.6): model output is validated
// against the embedded, versioned semlia.proposal-input/v1 JSON Schema
// before any domain call happens. ADR-0002 TDR-015 adopts
// github.com/santhosh-tekuri/jsonschema/v6 for exactly this condition
// ("校验 AI structured output"); the go.mod entry is already authorized.
//
// The gate is deliberately side-effect free: validation either rejects the
// payload with ErrAIOutputInvalid (no authorization evaluation, no agent-run
// lookup, no proposal write) or leaves the payload untouched for the
// authoring service. Raw prompts and credentials are unrepresentable: the
// schema has no field that could carry them.

// ProposalInputSchemaID is the versioned identifier of the embedded schema.
const ProposalInputSchemaID = "semlia.proposal-input/v1"

//go:embed proposal_input.v1.schema.json
var proposalInputSchemaV1 []byte

var (
	proposalInputOnce     sync.Once
	proposalInputSchema   *jsonschema.Schema
	proposalInputSetupErr error
)

func compileProposalInputSchema() {
	schema, err := jsonschema.UnmarshalJSON(bytes.NewReader(proposalInputSchemaV1))
	if err != nil {
		proposalInputSetupErr = fmt.Errorf("decode %s schema: %w", ProposalInputSchemaID, err)
		return
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(ProposalInputSchemaID, schema); err != nil {
		proposalInputSetupErr = fmt.Errorf("register %s schema: %w", ProposalInputSchemaID, err)
		return
	}
	compiled, err := compiler.Compile(ProposalInputSchemaID)
	if err != nil {
		proposalInputSetupErr = fmt.Errorf("compile %s schema: %w", ProposalInputSchemaID, err)
		return
	}
	proposalInputSchema = compiled
}

// AIOutputInvalidError carries the schema violations of a rejected agent
// payload. The HTTP boundary answers with 422 AI_OUTPUT_INVALID and a
// bounded violation summary; no raw payload content is echoed back.
type AIOutputInvalidError struct {
	Violations []string
}

func (err *AIOutputInvalidError) Error() string {
	return fmt.Sprintf("%s: %d schema violation(s)", ErrAIOutputInvalid.Error(), len(err.Violations))
}

func (err *AIOutputInvalidError) Unwrap() error { return ErrAIOutputInvalid }

// ErrAIOutputInvalid marks agent structured output that failed the versioned
// schema gate. Domain code never observes payloads carrying this error.
var ErrAIOutputInvalid = fmt.Errorf("agent structured output does not match %s", ProposalInputSchemaID)

// ValidateAgentProposalInput checks one raw AI structured-output document
// against semlia.proposal-input/v1. It performs no I/O and no domain calls,
// so a rejection costs zero writes anywhere in the system.
func ValidateAgentProposalInput(raw []byte) error {
	proposalInputOnce.Do(compileProposalInputSchema)
	if proposalInputSetupErr != nil {
		return fmt.Errorf("load %s schema: %w", ProposalInputSchemaID, proposalInputSetupErr)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return &AIOutputInvalidError{Violations: []string{"payload is not a valid JSON document"}}
	}
	if err := proposalInputSchema.Validate(document); err != nil {
		return &AIOutputInvalidError{Violations: schemaViolationSummaries(err)}
	}
	return nil
}

// schemaViolationSummaries flattens a jsonschema/v6 validation error into a
// bounded list of wire-safe leaf summaries (container nodes add nothing).
func schemaViolationSummaries(err error) []string {
	validationError, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{"payload does not match the schema"}
	}
	summaries := make([]string, 0, 4)
	collectViolationLeaves(validationError, &summaries)
	if len(summaries) == 0 {
		return []string{"payload does not match the schema"}
	}
	return summaries
}

func collectViolationLeaves(node *jsonschema.ValidationError, summaries *[]string) {
	if len(node.Causes) == 0 {
		summary := fmt.Sprintf("%s: %s", jsonPointer(node.InstanceLocation), node.Error())
		if len(summary) > 256 {
			summary = summary[:256]
		}
		*summaries = append(*summaries, summary)
		return
	}
	for _, cause := range node.Causes {
		if len(*summaries) >= 10 {
			return
		}
		collectViolationLeaves(cause, summaries)
	}
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return "payload"
	}
	return "/" + strings.Join(segments, "/")
}

// AgentPayloadHasAttribution reports whether a raw request body carries the
// agentAttribution block, so the caller knows the AI gate must run before any
// further decoding. The sniff decodes permissively: shape errors are the
// schema gate's business, not this function's.
func AgentPayloadHasAttribution(raw []byte) bool {
	var sniff struct {
		AgentAttribution json.RawMessage `json:"agentAttribution"`
	}
	return json.Unmarshal(raw, &sniff) == nil && len(sniff.AgentAttribution) > 0 &&
		string(sniff.AgentAttribution) != "null"
}
