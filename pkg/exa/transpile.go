package exa

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const (
	// unicodePrefix is prepended to encoded unicode identifiers to keep them CEL-compliant.
	unicodePrefix = "_u_"
)

var (
	// tokenRegex matches string literals (single/double quotes) or valid word identifiers.
	// We use the unicode property \p{L} to seamlessly support all international characters
	// (Korean, Japanese Hiragana/Katakana, Chinese Han, Cyrillic, Latin Extensions, etc.) as variables.
	tokenRegex = regexp.MustCompile(`'[^']*'|"[^"]*"|([\p{L}0-9_]+)`)

	// encodedIdentRegex validates if an identifier is a true encoded hex token
	encodedIdentRegex = regexp.MustCompile(`^` + unicodePrefix + `([0-9a-fA-F]+)$`)

	// errorObfuscationRegex scans error messages for any occurrence of transpiled hex variables.
	errorObfuscationRegex = regexp.MustCompile(unicodePrefix + `([0-9a-fA-F]+)`)
)

// encodeUnicodeIdent translates a Unicode identifier into an ASCII safe CEL variable.
func encodeUnicodeIdent(s string) string {
	if !hasUnicode(s) {
		return s
	}
	return unicodePrefix + hex.EncodeToString([]byte(s))
}

// decodeUnicodeIdent decodes the encoded ASCII safe variable back to its original Unicode string.
func decodeUnicodeIdent(s string) string {
	matches := encodedIdentRegex.FindStringSubmatch(s)
	if len(matches) < 2 {
		return s
	}
	
	decodedBytes, err := hex.DecodeString(matches[1])
	if err != nil {
		return s
	}
	return string(decodedBytes)
}

// hasUnicode reports whether the string contains any non-ASCII character.
func hasUnicode(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// cleanUnicodeString sanitizes Unicode inputs: normalizes to NFC to resolve NFD (macOS) mismatches,
// strips invisible formatting characters (like Zero-Width Space), and normalizes irregular spaces.
func cleanUnicodeString(s string) string {
	// 1. Normalize to NFC (Canonical Composition) to prevent NFD character decomposition mismatches.
	normalized := norm.NFC.String(s)

	// 2. Linear scan to strip invisible control characters and unify whitespace.
	var sb strings.Builder
	sb.Grow(len(normalized))
	for _, r := range normalized {
		switch r {
		case '\u200b', '\ufeff', '\u200c', '\u200d':
			// Silently ignore Zero-Width Spaces, BOMs, and Joiners
			continue
		case '\u00a0', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u202f', '\u205f', '\u3000':
			// Unify Non-Breaking Spaces, Ideographic Spaces, etc. to standard ASCII space
			sb.WriteRune(' ')
		default:
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

// NormalizeRequest pre-cleans all keys and expressions in the Request before parsing.
func NormalizeRequest(req Request) Request {
	cleaned := Request{
		Inputs: make(map[string]any, len(req.Inputs)),
		Policy: make([]Calculation, len(req.Policy)),
	}
	for k, v := range req.Inputs {
		cleaned.Inputs[cleanUnicodeString(k)] = v
	}
	for i, p := range req.Policy {
		cleaned.Policy[i] = Calculation{
			ID:         cleanUnicodeString(p.ID),
			Expression: cleanUnicodeString(p.Expression),
		}
	}
	return cleaned
}

// transpileExpression converts all Unicode identifiers inside an expression into ASCII safe ones,
// while completely preserving string literals.
func transpileExpression(expr string) string {
	return tokenRegex.ReplaceAllStringFunc(expr, func(match string) string {
		if isStringLiteral(match) {
			return match
		}
		return encodeUnicodeIdent(match)
	})
}

// isStringLiteral checks if the given token represents a string literal.
func isStringLiteral(s string) bool {
	if len(s) < 2 {
		return false
	}
	first := s[0]
	last := s[len(s)-1]
	return (first == '\'' && last == '\'') || (first == '"' && last == '"')
}

// NeedsTranspilation performs a quick check to see if the request contains any Unicode.
func NeedsTranspilation(req Request) bool {
	for k := range req.Inputs {
		if hasUnicode(k) {
			return true
		}
	}
	for _, p := range req.Policy {
		if hasUnicode(p.ID) || hasUnicode(p.Expression) {
			return true
		}
	}
	return false
}

// TranspileRequest converts all Unicode/Korean keys in Inputs and Expressions in Policy.
func TranspileRequest(req Request) Request {
	transpiled := Request{
		Inputs: make(map[string]any, len(req.Inputs)),
		Policy: make([]Calculation, len(req.Policy)),
	}
	for k, v := range req.Inputs {
		transpiled.Inputs[encodeUnicodeIdent(k)] = v
	}
	for i, p := range req.Policy {
		transpiled.Policy[i] = Calculation{
			ID:         encodeUnicodeIdent(p.ID),
			Expression: transpileExpression(p.Expression),
		}
	}
	return transpiled
}

// DeobfuscateError scans an error for transpiled hex variables (prefixed with `_u_`)
// and decodes them back to their original Unicode string, preserving Go's error wrapping.
func DeobfuscateError(err error) error {
	if err == nil {
		return nil
	}

	// Deobfuscate EvalError structurally if it matches
	if evalErr, ok := err.(*EvalError); ok {
		return &EvalError{
			ID:    decodeUnicodeIdent(evalErr.ID),
			Inner: DeobfuscateError(evalErr.Inner),
		}
	}

	msg := err.Error()
	deobfuscatedMsg := errorObfuscationRegex.ReplaceAllStringFunc(msg, func(match string) string {
		return decodeUnicodeIdent(match)
	})

	// Re-wrap and preserve sentinel error wrapping
	if strings.Contains(msg, ErrCircularDependency.Error()) {
		suffix := strings.TrimPrefix(deobfuscatedMsg, ErrCircularDependency.Error())
		suffix = strings.TrimPrefix(suffix, " at ")
		suffix = strings.TrimPrefix(suffix, ": ")
		if suffix != "" {
			return fmt.Errorf("%w at %s", ErrCircularDependency, suffix)
		}
		return ErrCircularDependency
	}

	if strings.Contains(msg, ErrDuplicateID.Error()) {
		suffix := strings.TrimPrefix(deobfuscatedMsg, ErrDuplicateID.Error())
		suffix = strings.TrimPrefix(suffix, ": ")
		if suffix != "" {
			return fmt.Errorf("%w: %s", ErrDuplicateID, suffix)
		}
		return ErrDuplicateID
	}

	return fmt.Errorf("%s", deobfuscatedMsg)
}
