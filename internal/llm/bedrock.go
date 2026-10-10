package llm

import (
	"context"
	"fmt"
)

// bedrockProvider is a placeholder for the AWS Bedrock Converse adapter. It is
// wired into the registry (so /models lists it and the design is complete) but
// not implemented in the test build:
//
//   - The Converse API needs SigV4 request signing via aws-sdk-go-v2, whose
//     current bedrockruntime module requires a newer Go than we target (1.18),
//     so it can't be fetched here.
//   - It authenticates with the END USER's AWS credentials (BYOK, their
//     account) and can't be exercised without a real AWS account, so it stays
//     deferred behind a clear error rather than shipping untested.
//
// When enabled: translate the internal representation to Converse's unified
// `toolUse`/`toolResult` blocks (the fourth wire schema the normalization layer
// already anticipates) and sign with aws-sdk-go-v2/bedrockruntime.
type bedrockProvider struct {
	id     string
	region string
}

func (p *bedrockProvider) Name() string { return p.id }

func (p *bedrockProvider) Stream(_ context.Context, _ Request, _ func(string)) (*Response, error) {
	return nil, fmt.Errorf(
		"bedrock provider %q is not implemented in this build — switch with /model to anthropic or an OpenAI-compatible provider",
		p.id)
}
