package ninerouter

import "bytes"

// normaliseJSON returns the JSON a model sent, without the wrappers some of
// them put around it.
//
// Two are common enough to be worth knowing by name. A markdown code fence is
// the model presenting its answer rather than returning it, and it arrives from
// any route and any model. A `<think></think>` block is a reasoning channel
// leaking into the content stream — on this project's gateway it has appeared
// empty, and only on one model id, which makes it the gateway's doing rather
// than the model's. They also arrive together.
//
// Both are removed because both are *delimited*: the opening and the closing
// marker are visible, so what comes off is exactly what was put on. That is the
// line this draws, and it is the same line the blueprint parser's comment
// argues for when it refuses to go "seeking for the outermost brace" — hunting
// for a likely-looking brace, or dropping leading lines that read like prose,
// eventually eats an answer. This cannot: anything it does not recognise passes
// through whole, and a genuinely malformed response fails exactly as before.
//
// Called where a response is parsed rather than where it is received, so the
// narration path never sees it and the transcript keeps the raw bytes.
func normaliseJSON(raw []byte) []byte {
	out := bytes.TrimSpace(raw)
	out = withoutThinkBlock(out)
	out = withoutCodeFence(out)
	return bytes.TrimSpace(out)
}

// withoutThinkBlock drops a leading <think>…</think>, closing tag required.
func withoutThinkBlock(raw []byte) []byte {
	const openTag, closeTag = "<think>", "</think>"
	if !bytes.HasPrefix(raw, []byte(openTag)) {
		return raw
	}
	end := bytes.Index(raw, []byte(closeTag))
	if end == -1 {
		// An unclosed tag is a response that was cut off, not one that was
		// wrapped. Nothing here can say where the reasoning stopped.
		return raw
	}
	return bytes.TrimSpace(raw[end+len(closeTag):])
}

// withoutCodeFence unwraps a ```lang … ``` fence, closing delimiter required.
func withoutCodeFence(raw []byte) []byte {
	const fence = "```"
	if !bytes.HasPrefix(raw, []byte(fence)) {
		return raw
	}
	// The rest of the opening line is the language tag, which is a label rather
	// than content whether it says json, JSON or nothing at all.
	body := raw[len(fence):]
	if nl := bytes.IndexByte(body, '\n'); nl != -1 {
		body = body[nl+1:]
	} else {
		body = nil
	}
	end := bytes.LastIndex(body, []byte(fence))
	if end == -1 {
		return raw
	}
	return bytes.TrimSpace(body[:end])
}
