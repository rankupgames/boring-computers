package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// defaultAgentGoal is used when the client doesn't pass ?goal=. The desktop is a
// minimal X session with a terminal on screen, so the default task is
// terminal-driven (the most reliable thing to demo).
const defaultAgentGoal = "Click on the terminal window to focus it, then type a command that prints a short friendly greeting and today's date, and run it. Then tell me what appeared."

const agentSystemPrompt = `You are operating a minimal Linux desktop by looking at screenshots and controlling the mouse and keyboard. This is a LIVE demo on a public website — real people are watching your screen right now.

Narrate as you work: before each action, write ONE short, friendly, first-person sentence about what you're doing (e.g. "Clicking the terminal to focus it." or "Typing the command now."). Keep it to a single sentence. Don't over-explain.

The desktop is minimal: it has a terminal (xterm) and a clock. To type in the terminal you must click it first to focus it. Coordinates are exact pixels on the screenshot.

Work efficiently — you have a limited number of steps. When the task is done, reply with one sentence starting with "Done:" and stop (do not call any more tools).`

// agentRuns counts in-flight agent loops (a cost guard alongside the step cap).
var agentRuns int32

func (s *Server) runAgent(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	id := r.PathValue("id")
	goal := strings.TrimSpace(r.URL.Query().Get("goal"))
	if goal == "" {
		goal = defaultAgentGoal
	}
	if len(goal) > 300 {
		goal = goal[:300]
	}

	// Dial the guest first so a non-desktop machine returns a clean HTTP error.
	guest, err := s.mgr.DialVsock(id, VsockPort)
	if err != nil {
		if err == ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		}
		return
	}
	defer guest.Close()

	cli, err := newRFBClient(guest)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "rfb: " + err.Error()})
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	send := func(typ, text string) {
		_ = conn.WriteJSON(map[string]string{"type": typ, "text": text})
	}

	if s.cfg.AnthropicKey == "" {
		send("error", "the agent isn't configured on this server")
		return
	}
	if n := atomic.AddInt32(&agentRuns, 1); int(n) > s.cfg.AgentMaxConcurrent {
		atomic.AddInt32(&agentRuns, -1)
		send("error", "too many agents are running right now — try again in a moment")
		return
	}
	defer atomic.AddInt32(&agentRuns, -1)

	// Detect the browser closing the socket (stop button / navigation away).
	stop := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(stop)
				return
			}
		}
	}()
	stopped := func() bool {
		select {
		case <-stop:
			return true
		default:
			return false
		}
	}

	send("say", "Taking a look at the screen…")
	shot, err := cli.Screenshot()
	if err != nil {
		send("error", "couldn't read the screen: "+err.Error())
		return
	}

	tool := map[string]any{
		"type":              "computer_20251124",
		"name":              "computer",
		"display_width_px":  cli.width,
		"display_height_px": cli.height,
		"display_number":    0,
	}
	messages := []json.RawMessage{userGoalMessage(goal, shot)}

	for step := 0; step < s.cfg.AgentMaxSteps; step++ {
		if stopped() {
			return
		}
		resp, err := callAnthropic(s.cfg, tool, messages)
		if err != nil {
			send("error", err.Error())
			return
		}
		messages = append(messages, assistantMessage(resp.Content))

		var results []json.RawMessage
		for _, raw := range resp.Content {
			var b blockHead
			if json.Unmarshal(raw, &b) != nil {
				continue
			}
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					send("say", t)
				}
			case "tool_use":
				if stopped() {
					return
				}
				send("action", describeAction(b.Input))
				out, errText := executeAction(cli, b.Input)
				results = append(results, toolResult(b.ID, out, errText))
			}
		}
		if len(results) == 0 {
			send("done", "")
			return
		}
		messages = append(messages, userToolResults(results))
	}
	send("done", "reached the step limit")
}

// executeAction performs a computer-use action and returns a fresh screenshot
// (or an error string to feed back as an is_error tool result).
func executeAction(cli *rfbClient, in map[string]any) ([]byte, string) {
	action, _ := in["action"].(string)
	coord := func(key string) (int, int, bool) {
		c, ok := in[key].([]any)
		if !ok || len(c) < 2 {
			return 0, 0, false
		}
		x, _ := c[0].(float64)
		y, _ := c[1].(float64)
		return int(x), int(y), true
	}

	switch action {
	case "screenshot":
		// fall through to capture
	case "mouse_move":
		if x, y, ok := coord("coordinate"); ok {
			cli.MoveMouse(x, y)
		}
	case "left_click":
		if x, y, ok := coord("coordinate"); ok {
			cli.Click(1, x, y)
		}
	case "right_click":
		if x, y, ok := coord("coordinate"); ok {
			cli.Click(4, x, y)
		}
	case "middle_click":
		if x, y, ok := coord("coordinate"); ok {
			cli.Click(2, x, y)
		}
	case "double_click":
		if x, y, ok := coord("coordinate"); ok {
			cli.Click(1, x, y)
			time.Sleep(90 * time.Millisecond)
			cli.Click(1, x, y)
		}
	case "triple_click":
		if x, y, ok := coord("coordinate"); ok {
			for i := 0; i < 3; i++ {
				cli.Click(1, x, y)
				time.Sleep(70 * time.Millisecond)
			}
		}
	case "left_click_drag":
		sx, sy, ok1 := coord("start_coordinate")
		ex, ey, ok2 := coord("coordinate")
		if ok1 && ok2 {
			cli.pointer(0, sx, sy)
			cli.pointer(1, sx, sy)
			time.Sleep(60 * time.Millisecond)
			cli.pointer(1, ex, ey)
			time.Sleep(60 * time.Millisecond)
			cli.pointer(0, ex, ey)
		}
	case "type":
		if t, ok := in["text"].(string); ok {
			cli.Type(t)
		}
	case "key":
		if t, ok := in["text"].(string); ok {
			if err := cli.Key(t); err != nil {
				return nil, err.Error()
			}
		}
	case "scroll":
		x, y, _ := coord("coordinate")
		dir, _ := in["scroll_direction"].(string)
		amt := 3
		if a, ok := in["scroll_amount"].(float64); ok {
			amt = int(a)
		}
		cli.Scroll(x, y, dir, amt)
	case "wait":
		d := 1.0
		if v, ok := in["duration"].(float64); ok {
			d = v
		}
		if d > 3 {
			d = 3
		}
		time.Sleep(time.Duration(d * float64(time.Second)))
	default:
		// Unknown/unsupported action (e.g. zoom, hold_key): just re-screenshot.
	}

	time.Sleep(350 * time.Millisecond) // let the UI settle
	shot, err := cli.Screenshot()
	if err != nil {
		return nil, "screenshot failed: " + err.Error()
	}
	return shot, ""
}

// describeAction produces a short human caption when the model doesn't narrate.
func describeAction(in map[string]any) string {
	action, _ := in["action"].(string)
	switch action {
	case "left_click", "double_click", "triple_click", "right_click", "middle_click", "mouse_move":
		if c, ok := in["coordinate"].([]any); ok && len(c) >= 2 {
			return fmt.Sprintf("%s at (%v, %v)", strings.ReplaceAll(action, "_", " "), int64(toF(c[0])), int64(toF(c[1])))
		}
	case "type":
		if t, ok := in["text"].(string); ok {
			return "typing: " + t
		}
	case "key":
		if t, ok := in["text"].(string); ok {
			return "pressing " + t
		}
	case "scroll":
		return "scrolling"
	case "screenshot":
		return "taking a screenshot"
	case "wait":
		return "waiting"
	}
	return action
}

func toF(v any) float64 { f, _ := v.(float64); return f }

// --- Anthropic Messages API (raw HTTP; computer-use beta) ---

type apiResp struct {
	Content    []json.RawMessage `json:"content"`
	StopReason string            `json:"stop_reason"`
}

type blockHead struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Text  string         `json:"text"`
	Input map[string]any `json:"input"`
}

func callAnthropic(cfg Config, tool map[string]any, messages []json.RawMessage) (*apiResp, error) {
	body := map[string]any{
		"model":         cfg.AgentModel,
		"max_tokens":    2048,
		"system":        agentSystemPrompt,
		"tools":         []any{tool},
		"messages":      messages,
		"output_config": map[string]any{"effort": "medium"},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", cfg.AnthropicKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "computer-use-2025-11-24")

	res, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("the AI is unreachable right now")
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(data, &e)
		if e.Error.Message != "" {
			return nil, fmt.Errorf("model error: %s", e.Error.Message)
		}
		return nil, fmt.Errorf("model http %d", res.StatusCode)
	}
	var out apiResp
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("bad model response")
	}
	return &out, nil
}

func imageBlock(png []byte) map[string]any {
	return map[string]any{
		"type": "image",
		"source": map[string]any{
			"type":       "base64",
			"media_type": "image/png",
			"data":       base64.StdEncoding.EncodeToString(png),
		},
	}
}

func userGoalMessage(goal string, shot []byte) json.RawMessage {
	m := map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "text", "text": "Your task: " + goal + "\n\nHere is the current screen:"},
			imageBlock(shot),
		},
	}
	b, _ := json.Marshal(m)
	return b
}

func assistantMessage(content []json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"role": "assistant", "content": content})
	return b
}

func toolResult(id string, shot []byte, errText string) json.RawMessage {
	if errText != "" {
		b, _ := json.Marshal(map[string]any{"type": "tool_result", "tool_use_id": id, "content": errText, "is_error": true})
		return b
	}
	b, _ := json.Marshal(map[string]any{"type": "tool_result", "tool_use_id": id, "content": []any{imageBlock(shot)}})
	return b
}

func userToolResults(results []json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"role": "user", "content": results})
	return b
}
