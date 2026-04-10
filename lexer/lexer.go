package lexer

import (
	"fmt"
)

type Lexer struct {
	source []rune
	pos    int
	line   int
	col    int
}

func New(source string) *Lexer {
	return &Lexer{
		source: []rune(source),
		pos:    0,
		line:   1,
		col:    1,
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.source) {
		return 0
	}
	return l.source[l.pos]
}

func (l *Lexer) peekNext() rune {
	if l.pos+1 >= len(l.source) {
		return 0
	}
	return l.source[l.pos+1]
}

func (l *Lexer) advance() rune {
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.source) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else if ch == '/' && l.peekNext() == '/' {
			for l.pos < len(l.source) && l.peek() != '\n' {
				l.advance()
			}
		} else if ch == '/' && l.peekNext() == '*' {
			l.skipBlockComment()
		} else {
			break
		}
	}
}

func (l *Lexer) skipBlockComment() {
	depth := 0
	for l.pos < len(l.source)-1 {
		if l.peek() == '/' && l.peekNext() == '*' {
			depth++
			l.advance()
			l.advance()
			continue
		}
		if l.peek() == '*' && l.peekNext() == '/' {
			depth--
			l.advance()
			l.advance()
			if depth <= 0 {
				return
			}
			continue
		}
		l.advance()
	}
}

func (l *Lexer) readString() Token {
	line, col := l.line, l.col
	l.advance()
	var result []rune
	for l.pos < len(l.source) && l.peek() != '"' {
		ch := l.advance()
		if ch == '\\' {
			esc := l.advance()
			switch esc {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			default:
				result = append(result, '\\', esc)
			}
		} else {
			result = append(result, ch)
		}
	}
	if l.pos < len(l.source) {
		l.advance()
	}
	return Token{Type: TOKEN_STRING, Literal: string(result), Line: line, Col: col}
}

func (l *Lexer) readNumber() Token {
	line, col := l.line, l.col
	start := l.pos
	if l.peek() == '0' && (l.peekNext() == 'x' || l.peekNext() == 'X') {
		l.advance()
		l.advance()
		for l.pos < len(l.source) && (isHexDigit(l.peek()) || l.peek() == '_') {
			l.advance()
		}
		lit := string(l.source[start:l.pos])
		return Token{Type: TOKEN_INT, Literal: normalizeNumberLiteral(lit), Line: line, Col: col}
	}
	if l.peek() == '0' && (l.peekNext() == 'b' || l.peekNext() == 'B') {
		l.advance()
		l.advance()
		for l.pos < len(l.source) && (l.peek() == '0' || l.peek() == '1' || l.peek() == '_') {
			l.advance()
		}
		lit := string(l.source[start:l.pos])
		return Token{Type: TOKEN_INT, Literal: normalizeNumberLiteral(lit), Line: line, Col: col}
	}
	isFloat := false
	for l.pos < len(l.source) {
		p := l.peek()
		if isDigit(p) || p == '_' {
			l.advance()
			continue
		}
		if p == '.' {
			if l.peekNext() == '.' {
				break
			}
			isFloat = true
			l.advance()
			continue
		}
		break
	}
	lit := normalizeNumberLiteral(string(l.source[start:l.pos]))
	if isFloat {
		return Token{Type: TOKEN_FLOAT, Literal: lit, Line: line, Col: col}
	}
	return Token{Type: TOKEN_INT, Literal: lit, Line: line, Col: col}
}

func normalizeNumberLiteral(lit string) string {
	out := make([]rune, 0, len(lit))
	for _, r := range lit {
		if r != '_' {
			out = append(out, r)
		}
	}
	return string(out)
}

func (l *Lexer) readIdent() Token {
	line, col := l.line, l.col
	start := l.pos
	for l.pos < len(l.source) && (isLetter(l.peek()) || isDigit(l.peek())) {
		l.advance()
	}
	lit := string(l.source[start:l.pos])
	typ := LookupIdent(lit)
	return Token{Type: typ, Literal: lit, Line: line, Col: col}
}

func (l *Lexer) readTemplateString() Token {
	line, col := l.line, l.col
	l.advance()
	start := l.pos
	for l.pos < len(l.source) && l.peek() != '`' {
		if l.peek() == '\\' && l.pos+1 < len(l.source) {
			l.advance()
		}
		l.advance()
	}
	lit := string(l.source[start:l.pos])
	if l.pos < len(l.source) && l.peek() == '`' {
		l.advance()
	}
	return Token{Type: TOKEN_TEMPLATE, Literal: lit, Line: line, Col: col}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.source) {
		return Token{Type: TOKEN_EOF, Literal: "", Line: l.line, Col: l.col}
	}

	line, col := l.line, l.col
	ch := l.peek()

	if ch == '\n' {
		l.advance()
		return Token{Type: TOKEN_NEWLINE, Literal: "\n", Line: line, Col: col}
	}

	if ch == '"' {
		return l.readString()
	}
	if ch == '`' {
		return l.readTemplateString()
	}

	if isDigit(ch) {
		return l.readNumber()
	}

	if isLetter(ch) {
		return l.readIdent()
	}

	l.advance()

	switch ch {
	case '+':
		if l.peek() == '+' {
			l.advance()
			return Token{Type: TOKEN_PLUSPLUS, Literal: "++", Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_PLUSEQ, Literal: "+=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_PLUS, Literal: "+", Line: line, Col: col}
	case '-':
		if l.peek() == '-' {
			l.advance()
			return Token{Type: TOKEN_MINUSMINUS, Literal: "--", Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_MINUSEQ, Literal: "-=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_MINUS, Literal: "-", Line: line, Col: col}
	case '*':
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_STAREQ, Literal: "*=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_STAR, Literal: "*", Line: line, Col: col}
	case '/':
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_SLASHEQ, Literal: "/=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_SLASH, Literal: "/", Line: line, Col: col}
	case '%':
		return Token{Type: TOKEN_PERCENT, Literal: "%", Line: line, Col: col}
	case '=':
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_EQ, Literal: "==", Line: line, Col: col}
		}
		return Token{Type: TOKEN_ASSIGN, Literal: "=", Line: line, Col: col}
	case '!':
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_NEQ, Literal: "!=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_BANG, Literal: "!", Line: line, Col: col}
	case '<':
		if l.peek() == '<' {
			l.advance()
			return Token{Type: TOKEN_SHL, Literal: "<<", Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_LTE, Literal: "<=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_LT, Literal: "<", Line: line, Col: col}
	case '>':
		if l.peek() == '>' {
			l.advance()
			return Token{Type: TOKEN_SHR, Literal: ">>", Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_GTE, Literal: ">=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_GT, Literal: ">", Line: line, Col: col}
	case '&':
		if l.peek() == '&' {
			l.advance()
			return Token{Type: TOKEN_AND, Literal: "&&", Line: line, Col: col}
		}
		return Token{Type: TOKEN_AMP, Literal: "&", Line: line, Col: col}
	case '|':
		if l.peek() == '|' {
			l.advance()
			return Token{Type: TOKEN_OR, Literal: "||", Line: line, Col: col}
		}
		return Token{Type: TOKEN_PIPE, Literal: "|", Line: line, Col: col}
	case '^':
		return Token{Type: TOKEN_CARET, Literal: "^", Line: line, Col: col}
	case ':':
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TOKEN_WALRUS, Literal: ":=", Line: line, Col: col}
		}
		return Token{Type: TOKEN_COLON, Literal: ":", Line: line, Col: col}
	case '(':
		return Token{Type: TOKEN_LPAREN, Literal: "(", Line: line, Col: col}
	case ')':
		return Token{Type: TOKEN_RPAREN, Literal: ")", Line: line, Col: col}
	case '{':
		return Token{Type: TOKEN_LBRACE, Literal: "{", Line: line, Col: col}
	case '}':
		return Token{Type: TOKEN_RBRACE, Literal: "}", Line: line, Col: col}
	case '[':
		return Token{Type: TOKEN_LBRACKET, Literal: "[", Line: line, Col: col}
	case ']':
		return Token{Type: TOKEN_RBRACKET, Literal: "]", Line: line, Col: col}
	case ',':
		return Token{Type: TOKEN_COMMA, Literal: ",", Line: line, Col: col}
	case ';':
		return Token{Type: TOKEN_SEMICOLON, Literal: ";", Line: line, Col: col}
	case '.':
		if l.peek() == '.' {
			l.advance()
			if l.peek() == '.' {
				l.advance()
				return Token{Type: TOKEN_ELLIPSIS, Literal: "...", Line: line, Col: col}
			}
			return Token{Type: TOKEN_DOTDOT, Literal: "..", Line: line, Col: col}
		}
		return Token{Type: TOKEN_DOT, Literal: ".", Line: line, Col: col}
	case '?':
		return Token{Type: TOKEN_QUESTION, Literal: "?", Line: line, Col: col}
	}

	return Token{Type: TOKEN_ILLEGAL, Literal: fmt.Sprintf("%c", ch), Line: line, Col: col}
}

func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TOKEN_EOF {
			break
		}
	}
	return tokens
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
