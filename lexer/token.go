package lexer

type TokenType string

const (
	TOKEN_IDENT      TokenType = "IDENT"
	TOKEN_INT        TokenType = "INT"
	TOKEN_FLOAT      TokenType = "FLOAT"
	TOKEN_STRING     TokenType = "STRING"
	TOKEN_BOOL       TokenType = "BOOL"
	TOKEN_FUNC       TokenType = "func"
	TOKEN_VAR        TokenType = "var"
	TOKEN_CONST      TokenType = "const"
	TOKEN_RETURN     TokenType = "return"
	TOKEN_IF         TokenType = "if"
	TOKEN_ELSE       TokenType = "else"
	TOKEN_FOR        TokenType = "for"
	TOKEN_IN         TokenType = "in"
	TOKEN_BREAK      TokenType = "break"
	TOKEN_CONTINUE   TokenType = "continue"
	TOKEN_SWITCH     TokenType = "switch"
	TOKEN_CASE       TokenType = "case"
	TOKEN_DEFAULT    TokenType = "default"
	TOKEN_ENUM       TokenType = "enum"
	TOKEN_TYPE       TokenType = "type"
	TOKEN_TRY        TokenType = "try"
	TOKEN_CATCH      TokenType = "catch"
	TOKEN_FINALLY    TokenType = "finally"
	TOKEN_THROW      TokenType = "throw"
	TOKEN_DEFER      TokenType = "defer"
	TOKEN_ASYNC      TokenType = "async"
	TOKEN_AWAIT      TokenType = "await"
	TOKEN_IMPORT     TokenType = "import"
	TOKEN_FROM       TokenType = "from"
	TOKEN_TRUE       TokenType = "true"
	TOKEN_FALSE      TokenType = "false"
	TOKEN_NIL        TokenType = "nil"
	TOKEN_STRUCT     TokenType = "struct"
	TOKEN_ASSIGN     TokenType = "="
	TOKEN_PLUS       TokenType = "+"
	TOKEN_MINUS      TokenType = "-"
	TOKEN_STAR       TokenType = "*"
	TOKEN_SLASH      TokenType = "/"
	TOKEN_PERCENT    TokenType = "%"
	TOKEN_EQ         TokenType = "=="
	TOKEN_NEQ        TokenType = "!="
	TOKEN_LT         TokenType = "<"
	TOKEN_GT         TokenType = ">"
	TOKEN_LTE        TokenType = "<="
	TOKEN_GTE        TokenType = ">="
	TOKEN_AND        TokenType = "&&"
	TOKEN_OR         TokenType = "||"
	TOKEN_AMP        TokenType = "&"
	TOKEN_PIPE       TokenType = "|"
	TOKEN_CARET      TokenType = "^"
	TOKEN_SHL        TokenType = "<<"
	TOKEN_SHR        TokenType = ">>"
	TOKEN_BANG       TokenType = "!"
	TOKEN_PLUSEQ     TokenType = "+="
	TOKEN_MINUSEQ    TokenType = "-="
	TOKEN_STAREQ     TokenType = "*="
	TOKEN_SLASHEQ    TokenType = "/="
	TOKEN_WALRUS     TokenType = ":="
	TOKEN_PLUSPLUS   TokenType = "++"
	TOKEN_MINUSMINUS TokenType = "--"
	TOKEN_LPAREN     TokenType = "("
	TOKEN_RPAREN     TokenType = ")"
	TOKEN_LBRACE     TokenType = "{"
	TOKEN_RBRACE     TokenType = "}"
	TOKEN_LBRACKET   TokenType = "["
	TOKEN_RBRACKET   TokenType = "]"
	TOKEN_COMMA      TokenType = ","
	TOKEN_SEMICOLON  TokenType = ";"
	TOKEN_COLON      TokenType = ":"
	TOKEN_DOT        TokenType = "."
	TOKEN_DOTDOT     TokenType = ".."
	TOKEN_ELLIPSIS   TokenType = "..."
	TOKEN_QUESTION   TokenType = "?"
	TOKEN_TEMPLATE   TokenType = "TEMPLATE"
	TOKEN_EOF        TokenType = "EOF"
	TOKEN_NEWLINE    TokenType = "NEWLINE"
	TOKEN_ILLEGAL    TokenType = "ILLEGAL"
	TOKEN_AS         TokenType = "as"
)

var keywords = map[string]TokenType{
	"func":     TOKEN_FUNC,
	"var":      TOKEN_VAR,
	"const":    TOKEN_CONST,
	"return":   TOKEN_RETURN,
	"if":       TOKEN_IF,
	"else":     TOKEN_ELSE,
	"for":      TOKEN_FOR,
	"in":       TOKEN_IN,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"switch":   TOKEN_SWITCH,
	"case":     TOKEN_CASE,
	"default":  TOKEN_DEFAULT,
	"enum":     TOKEN_ENUM,
	"type":     TOKEN_TYPE,
	"try":      TOKEN_TRY,
	"catch":    TOKEN_CATCH,
	"finally":  TOKEN_FINALLY,
	"throw":    TOKEN_THROW,
	"defer":    TOKEN_DEFER,
	"async":    TOKEN_ASYNC,
	"await":    TOKEN_AWAIT,
	"import":   TOKEN_IMPORT,
	"from":     TOKEN_FROM,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"nil":      TOKEN_NIL,
	"struct":   TOKEN_STRUCT,
	"as":       TOKEN_AS,
}

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}
