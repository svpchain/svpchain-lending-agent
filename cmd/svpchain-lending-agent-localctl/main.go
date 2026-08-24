// Command svpchain-lending-agent-localctl authenticates a local operator to a
// running lending agent. The agent signs and broadcasts the registry message.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/svpchain/svpchain-lending-agent/internal/mcp/signer"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

const (
	authSkill      = toolbridge.SkillAuth
	executionSkill = toolbridge.SkillExecution
)

type response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type identity struct {
	Operator   string `json:"operator"`
	AgentID    string `json:"agent_id"`
	Registered bool   `json:"registered"`
	Endpoint   string `json:"endpoint"`
	CardHash   string `json:"card_hash"`
}

type challenge struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
}

type verify struct {
	BearerToken string `json:"bearer_token"`
	Owner       string `json:"owner"`
}

type txResult struct {
	TxHash string `json:"tx_hash"`
	Code   uint32 `json:"code"`
	RawLog string `json:"raw_log"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "svpchain-lending-agent-localctl: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	action := flag.String("action", "", "address, register, or update")
	agentURL := flag.String("agent-url", "", "running agent base URL")
	keyFile := flag.String("key-file", "", "32-byte hex operator private key file")
	bond := flag.String("bond", "", "initial registration bond, e.g. 5000asvp")
	flag.Parse()
	if *action != "address" && *action != "register" && *action != "update" {
		return fmt.Errorf("--action must be address, register, or update")
	}
	if *keyFile == "" {
		return fmt.Errorf("--key-file is required")
	}
	if *action != "address" && *agentURL == "" {
		return fmt.Errorf("--agent-url is required for %s", *action)
	}
	if *action == "update" && *bond != "" {
		return fmt.Errorf("--bond is only valid with --action register")
	}
	rawKey, err := os.ReadFile(*keyFile)
	if err != nil {
		return fmt.Errorf("read operator key: %w", err)
	}
	priv, err := signer.ParsePrivKey(string(rawKey))
	if err != nil {
		return fmt.Errorf("parse operator key: %w", err)
	}
	operator := signer.DeriveAddress(priv)
	if *action == "address" {
		fmt.Println(operator)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := newClient(ctx, strings.TrimRight(*agentURL, "/"))
	if err != nil {
		return err
	}
	defer client.Destroy()
	currentReply, err := call(ctx, client, "agent_identity", map[string]any{}, "")
	if err != nil {
		return fmt.Errorf("query agent identity: %w", err)
	}
	var current identity
	if err := json.Unmarshal(currentReply.Result, &current); err != nil {
		return fmt.Errorf("decode agent identity: %w", err)
	}
	if current.Operator != operator {
		return fmt.Errorf("key-file operator %s does not match running agent operator %s", operator, current.Operator)
	}
	if (*action == "register" && current.Registered) || (*action == "update" && !current.Registered) {
		return fmt.Errorf("agent %s is already registered=%t; use the other lifecycle action", current.AgentID, current.Registered)
	}

	challengeReply, err := call(ctx, client, "auth_challenge", map[string]any{"owner": operator}, "")
	if err != nil {
		return fmt.Errorf("request auth challenge: %w", err)
	}
	var authChallenge challenge
	if err := json.Unmarshal(challengeReply.Result, &authChallenge); err != nil {
		return fmt.Errorf("decode auth challenge: %w", err)
	}
	signature, err := priv.Sign([]byte(authChallenge.Challenge))
	if err != nil {
		return fmt.Errorf("sign auth challenge: %w", err)
	}
	verifyReply, err := call(ctx, client, "auth_verify", map[string]any{
		"nonce": authChallenge.Nonce, "signature": base64.StdEncoding.EncodeToString(signature),
	}, "")
	if err != nil {
		return fmt.Errorf("verify operator authentication: %w", err)
	}
	var authVerify verify
	if err := json.Unmarshal(verifyReply.Result, &authVerify); err != nil {
		return fmt.Errorf("decode auth verification: %w", err)
	}
	if authVerify.Owner != operator || authVerify.BearerToken == "" {
		return fmt.Errorf("operator authentication returned an invalid identity")
	}
	args := map[string]any{}
	if *action == "register" && *bond != "" {
		args["bond"] = map[string]string{"amount": leadingDigits(*bond), "denom": strings.TrimPrefix(*bond, leadingDigits(*bond))}
	}
	result, err := call(ctx, client, "agent_self_"+*action, args, authVerify.BearerToken)
	if err != nil {
		return fmt.Errorf("%s: %w", *action, err)
	}
	var tx txResult
	if err := json.Unmarshal(result.Result, &tx); err != nil {
		return fmt.Errorf("decode %s transaction: %w", *action, err)
	}
	if tx.Code != 0 {
		return fmt.Errorf("%s transaction %s was rejected with code %d: %s", *action, tx.TxHash, tx.Code, tx.RawLog)
	}
	pretty, _ := json.MarshalIndent(result.Result, "", "  ")
	fmt.Printf("%s accepted by the local chain\nagent id:  %s\nendpoint:  %s\ncard hash: %s\n%s\n", *action, current.AgentID, current.Endpoint, current.CardHash, pretty)
	return nil
}

func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

func newClient(ctx context.Context, agentURL string) (*a2aclient.Client, error) {
	endpoint := a2a.NewAgentInterface(agentURL+"/invoke", a2a.TransportProtocolJSONRPC)
	return a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{endpoint}, a2aclient.WithJSONRPCTransport(&http.Client{Timeout: 30 * time.Second}))
}

func call(ctx context.Context, client *a2aclient.Client, tool string, args map[string]any, bearer string) (response, error) {
	envelope := map[string]any{"skill": skillForTool(tool), "tool": tool, "args": args}
	if bearer != "" {
		envelope["bearer"] = bearer
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return response{}, err
	}
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(string(body)))})
	if err != nil {
		return response{}, err
	}
	text := resultText(result)
	if text == "" {
		return response{}, fmt.Errorf("agent returned no message response")
	}
	var reply response
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return response{}, err
	}
	if !reply.OK {
		return response{}, fmt.Errorf("%s", reply.Error)
	}
	return reply, nil
}

func skillForTool(tool string) string {
	switch tool {
	case "auth_challenge", "auth_verify":
		return authSkill
	default:
		return executionSkill
	}
}

func resultText(result a2a.SendMessageResult) string {
	messageText := func(message *a2a.Message) string {
		if message == nil {
			return ""
		}
		var text strings.Builder
		for _, part := range message.Parts {
			if part != nil {
				text.WriteString(part.Text())
			}
		}
		return text.String()
	}
	switch value := result.(type) {
	case *a2a.Message:
		return messageText(value)
	case *a2a.Task:
		if text := messageText(value.Status.Message); text != "" {
			return text
		}
		for i := len(value.History) - 1; i >= 0; i-- {
			if text := messageText(value.History[i]); text != "" {
				return text
			}
		}
	}
	return ""
}
