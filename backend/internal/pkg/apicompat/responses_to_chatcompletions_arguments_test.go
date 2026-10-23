package apicompat

import (
	"reflect"
	"testing"
)

func TestResponsesEventToChatChunks_CompletedToolArguments(t *testing.T) {
	added := func(index int, kind string) ResponsesStreamEvent {
		return ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: index, Item: &ResponsesOutput{Type: kind, CallID: "call-test", Name: "read"}}
	}
	delta := func(index int, value string) ResponsesStreamEvent {
		return ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: index, Delta: value}
	}
	done := func(index int, value string) ResponsesStreamEvent {
		return ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: index, Arguments: value}
	}
	itemDone := func(index int, value string) ResponsesStreamEvent {
		return ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: index, Item: &ResponsesOutput{Type: "function_call", CallID: "call-test", Name: "read", Arguments: value}}
	}
	completed := func(output ...ResponsesOutput) ResponsesStreamEvent {
		return ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: output}}
	}
	const full = `{"path":"image.png"}`
	cases := []struct {
		name   string
		events []ResponsesStreamEvent
		want   map[int]string
		finish string
	}{
		{name: "parallel done-only calls retain separate indices", events: []ResponsesStreamEvent{
			added(2, "function_call"), added(5, "function_call"), added(7, "function_call"),
			done(7, `{"path":"3.png"}`), done(2, `{"path":"1.png"}`), done(5, `{"path":"2.png"}`), completed(),
		}, want: map[int]string{0: `{"path":"1.png"}`, 1: `{"path":"2.png"}`, 2: `{"path":"3.png"}`}, finish: "tool_calls"},
		{name: "complete delta plus repeated snapshots is emitted once", events: []ResponsesStreamEvent{
			added(0, "function_call"), delta(0, `{"path":`), delta(0, `"image.png"}`), done(0, full), done(0, full), itemDone(0, full),
			completed(ResponsesOutput{Type: "function_call", CallID: "call-test", Name: "read", Arguments: full}),
		}, want: map[int]string{0: full}, finish: "tool_calls"},
		{name: "partial delta completed by done", events: []ResponsesStreamEvent{added(0, "function_call"), delta(0, `{"path":`), done(0, full), done(0, full)}, want: map[int]string{0: full}},
		{name: "output item done fallback", events: []ResponsesStreamEvent{added(0, "function_call"), itemDone(0, full), itemDone(0, full)}, want: map[int]string{0: full}},
		{name: "output item done can register a missing header", events: []ResponsesStreamEvent{itemDone(4, full), itemDone(4, full)}, want: map[int]string{0: full}},
		{name: "terminal output fallback preserves real output positions", events: []ResponsesStreamEvent{
			added(1, "function_call"), delta(1, `{"path":`), completed(ResponsesOutput{Type: "reasoning"}, ResponsesOutput{Type: "function_call", CallID: "call-test", Name: "read", Arguments: full}),
		}, want: map[int]string{0: full}, finish: "tool_calls"},
		{name: "terminal output alone registers call", events: []ResponsesStreamEvent{completed(ResponsesOutput{Type: "message"}, ResponsesOutput{Type: "function_call", CallID: "call-test", Name: "read", Arguments: full})}, want: map[int]string{0: full}, finish: "tool_calls"},
		{name: "custom input done-only", events: []ResponsesStreamEvent{added(0, "custom_tool_call"), {Type: "response.custom_tool_call_input.done", OutputIndex: 0, Input: "*** Begin Patch"}}, want: map[int]string{0: "*** Begin Patch"}},
		{name: "custom input delta plus snapshot", events: []ResponsesStreamEvent{added(0, "custom_tool_call"), {Type: "response.custom_tool_call_input.delta", OutputIndex: 0, Delta: "*** Begin"}, {Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{Type: "custom_tool_call", CallID: "call-test", Name: "read", Input: "*** Begin Patch"}}}, want: map[int]string{0: "*** Begin Patch"}},
		{name: "incompatible snapshot cannot be appended to sent bytes", events: []ResponsesStreamEvent{added(0, "function_call"), delta(0, full), done(0, `{"path":"other.png"}`)}, want: map[int]string{0: full}},
		{name: "unknown argument event does not invent an unnamed tool", events: []ResponsesStreamEvent{done(9, full)}, want: map[int]string{}},
		{name: "duplicate added retains index", events: []ResponsesStreamEvent{added(0, "function_call"), added(0, "function_call"), done(0, full)}, want: map[int]string{0: full}},
		{name: "finalized streams ignore late tool events", events: []ResponsesStreamEvent{completed(), itemDone(0, full), completed()}, want: map[int]string{}, finish: "stop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := NewResponsesEventToChatState()
			got := map[int]string{}
			headers := map[int]int{}
			finishCount := 0
			finished := false
			for i := range tc.events {
				for _, chunk := range ResponsesEventToChatChunks(&tc.events[i], state) {
					for _, choice := range chunk.Choices {
						for _, call := range choice.Delta.ToolCalls {
							if finished {
								t.Fatal("tool arguments emitted after finish")
							}
							if call.Index == nil {
								t.Fatal("missing tool index")
							}
							index := *call.Index
							got[index] += call.Function.Arguments
							if call.ID != "" {
								headers[index]++
								if call.Function.Name != "read" {
									t.Errorf("wrong tool name: %q", call.Function.Name)
								}
							}
						}
						if choice.FinishReason != nil {
							finishCount++
							finished = true
							if *choice.FinishReason != tc.finish {
								t.Errorf("finish=%q, want %q", *choice.FinishReason, tc.finish)
							}
						}
					}
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("arguments=%#v, want %#v", got, tc.want)
			}
			for index := range tc.want {
				if headers[index] != 1 {
					t.Errorf("index %d emitted %d headers, want 1", index, headers[index])
				}
			}
			if tc.finish != "" && finishCount != 1 {
				t.Errorf("finish count=%d, want 1", finishCount)
			}
		})
	}
}
