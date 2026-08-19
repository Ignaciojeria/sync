package application

import "strings"

func mergeAssistantDelta(current, delta string) string {
	if delta == "" {
		return current
	}
	if current == "" {
		return delta
	}
	if strings.HasPrefix(delta, current) {
		return delta
	}
	if strings.HasSuffix(current, delta) {
		return current
	}
	if overlap := longestOverlapSuffixPrefix(current, delta); overlap > 0 {
		return current + delta[overlap:]
	}
	return current + delta
}

func longestOverlapSuffixPrefix(a, b string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for n := max; n > 0; n-- {
		if a[len(a)-n:] == b[:n] {
			return n
		}
	}
	return 0
}

func visibleAssistantText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	for {
		start := strings.Index(text, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(text[start+len("<think>"):], "</think>")
		if end < 0 {
			// ponytail: <think> sin </think> (stream cortado
			// mid-reasoning, modelo inlineó un tag suelto, etc).
			// El código viejo hacía `text = text[:start]`, que
			// borra TODO desde el <think> hasta el final —
			// incluyendo la respuesta visible que vino después.
			// Eso es el bug que el user reportó como "texto
			// español recortado" en sesiones donde el stream se
			// cortó. La fix: salir del loop sin chappear, y
			// dejar que el ReplaceAll de abajo limpie el tag
			// huérfano. El usuario ve el texto restante, aunque
			// incluya algo de razonamiento parcial. Es mejor que
			// perder la respuesta.
			break
		}
		end += start + len("<think>")
		text = text[:start] + text[end+len("</think>"):]
	}
	text = strings.ReplaceAll(text, "<think>", "")
	text = strings.ReplaceAll(text, "</think>", "")
	return strings.TrimSpace(text)
}
