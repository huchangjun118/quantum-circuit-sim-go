package qsim

import (
	"fmt"
	"strconv"
	"strings"
)

// Circuit 表示一份解析完成、通过校验的量子线路。
type Circuit struct {
	NQubits int
	Gates   []*Gate
}

// Parse 解析线路描述文本。
//
// 语法（每行一条指令，# 之后为注释，空行忽略）：
//
//	qubits 3            # 必须在任何门之前声明，且只能声明一次
//	h 0                 # 单比特门
//	cx 0,1              # 受控门：控制位 0，目标 1
//	ccx 0,1,2           # 双控制位（Toffoli）
//	rx(90) 0            # 参数门，角度单位为度
//	swap 0,1            # 多比特门
//	measure 1           # 测量比特 1（立即坍缩）
//	m 0,1               # 依次测量多个比特
//	measure             # 不带参数 = 测量全部比特
//
// 任何无法识别的指令都返回 *ParseError（含行列号），绝不静默跳过。
func Parse(src string) (*Circuit, error) {
	c := &Circuit{}
	declared := false

	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for idx, rawLine := range rawLines {
		lineNo := idx + 1

		// 去除注释。
		line := rawLine
		if h := strings.IndexByte(line, '#'); h >= 0 {
			line = line[:h]
		}
		// 跳过纯空白行。
		headStart := firstNonSpace(line)
		if headStart == len(line) {
			continue
		}
		col := headStart + 1
		trimmedRight := strings.TrimRight(line, " \t")
		content := line[headStart:]

		head, params, paramsOK, rest, perr := splitHead(content)
		if perr != "" {
			return nil, &ParseError{
				Line: lineNo, Column: col,
				Source:  strings.TrimSpace(trimmedRight),
				Op:      head,
				Message: perr,
			}
		}

		name := strings.ToUpper(head)

		// qubits 声明。
		if name == "QUBITS" {
			if declared {
				return nil, &ConflictError{
					Line: lineNo, Op: "qubits",
					Message: `"qubits" may be declared only once`,
				}
			}
			if paramsOK {
				return nil, &ParseError{
					Line: lineNo, Column: col, Op: "qubits",
					Source:  strings.TrimSpace(trimmedRight),
					Message: `unexpected parenthesized parameters on "qubits"`,
				}
			}
			toks, terr := tokenizeOperands(rest, headStart+len(head))
			if terr != nil {
				return nil, &ParseError{
					Line: lineNo, Column: terr.col, Op: "qubits",
					Source:  strings.TrimSpace(trimmedRight),
					Message: terr.msg,
				}
			}
			if len(toks) != 1 {
				return nil, &ParseError{
					Line: lineNo, Column: col, Op: "qubits",
					Source:  strings.TrimSpace(trimmedRight),
					Message: fmt.Sprintf(`"qubits" expects exactly one positive integer, got %d token(s)`, len(toks)),
				}
			}
			n, err := strconv.Atoi(toks[0].text)
			if err != nil || n < 1 {
				return nil, &ParseError{
					Line: lineNo, Column: toks[0].col, Op: "qubits",
					Source:  strings.TrimSpace(trimmedRight),
					Message: fmt.Sprintf("qubit count must be a positive integer, got %q", toks[0].text),
				}
			}
			c.NQubits = n
			declared = true
			continue
		}

		// 门之前必须先声明 qubits。
		if !declared {
			return nil, &ConflictError{
				Line: lineNo, Op: head,
				Message: `instruction appears before "qubits <n>" declaration`,
			}
		}

		// 测量指令。
		if name == "M" || name == "MEASURE" {
			if paramsOK {
				return nil, &ParseError{
					Line: lineNo, Column: col, Op: head,
					Source:  strings.TrimSpace(trimmedRight),
					Message: "measure does not take parenthesized parameters",
				}
			}
			toks, terr := tokenizeOperands(rest, headStart+len(head))
			if terr != nil {
				return nil, &ParseError{
					Line: lineNo, Column: terr.col, Op: head,
					Source:  strings.TrimSpace(trimmedRight),
					Message: terr.msg,
				}
			}
			var targets []int
			for _, t := range toks {
				q, err := strconv.Atoi(t.text)
				if err != nil {
					return nil, &ParseError{
						Line: lineNo, Column: t.col, Op: head,
						Source:  strings.TrimSpace(trimmedRight),
						Message: fmt.Sprintf("operand must be an integer qubit index, got %q", t.text),
					}
				}
				targets = append(targets, q)
			}
			if len(targets) == 0 {
				targets = make([]int, c.NQubits)
				for q := range targets {
					targets[q] = q
				}
			}
			for _, q := range targets {
				c.Gates = append(c.Gates, &Gate{
					Name:      "MEASURE",
					Targets:   []int{q},
					IsMeasure: true,
					Line:      lineNo,
					Column:    col,
					Raw:       strings.TrimSpace(trimmedRight),
				})
			}
			continue
		}

		// 受控/普通门。
		base, nControls, ok := GateInfo(name)
		if !ok {
			return nil, &ParseError{
				Line: lineNo, Column: col, Op: head,
				Source:  strings.TrimSpace(trimmedRight),
				Message: fmt.Sprintf("unrecognized instruction %q", head),
			}
		}
		arity := TargetArity(base)
		wantOps := nControls + arity

		var pvals []float64
		if ParamCount(base) > 0 {
			if !paramsOK {
				return nil, &MissingParamError{
					Op: head, Line: lineNo, Param: "theta",
					Message: fmt.Sprintf("gate %s requires a parameter, use e.g. %s(90)", head, base),
				}
			}
			pv, perr2 := parseParams(params, ParamCount(base))
			if perr2 != "" {
				return nil, &MissingParamError{
					Op: head, Line: lineNo, Param: "theta", Message: perr2,
				}
			}
			pvals = pv
		} else if paramsOK {
			return nil, &ParseError{
				Line: lineNo, Column: col, Op: head,
				Source:  strings.TrimSpace(trimmedRight),
				Message: fmt.Sprintf("gate %s does not take parameters", head),
			}
		}

		toks, terr := tokenizeOperands(rest, headStart+len(head))
		if terr != nil {
			return nil, &ParseError{
				Line: lineNo, Column: terr.col, Op: head,
				Source:  strings.TrimSpace(trimmedRight),
				Message: terr.msg,
			}
		}
		if len(toks) != wantOps {
			return nil, &ParseError{
				Line: lineNo, Column: col, Op: head,
				Source:  strings.TrimSpace(trimmedRight),
				Message: fmt.Sprintf("gate %s expects %d operand(s), got %d", head, wantOps, len(toks)),
			}
		}
		ops := make([]int, wantOps)
		for i, t := range toks {
			q, err := strconv.Atoi(t.text)
			if err != nil {
				return nil, &ParseError{
					Line: lineNo, Column: t.col, Op: head,
					Source:  strings.TrimSpace(trimmedRight),
					Message: fmt.Sprintf("operand must be an integer qubit index, got %q", t.text),
				}
			}
			ops[i] = q
		}

		c.Gates = append(c.Gates, &Gate{
			Name:     base,
			Controls: ops[:nControls],
			Targets:  ops[nControls:],
			Params:   pvals,
			Line:     lineNo,
			Column:   col,
			Raw:      strings.TrimSpace(trimmedRight),
		})
	}

	if !declared {
		return nil, &ConflictError{
			Line: 1, Column: 1,
			Message: `missing "qubits <n>" declaration; it must precede all gate instructions`,
		}
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// splitHead 从指令内容（已去掉前导空白）中分离：
// head（门名/关键字）、括号内参数原文、是否存在括号、剩余文本。
// 形如 "rx(90) 0,1" -> ("rx", "90", true, " 0,1", "")。
func splitHead(content string) (head, params string, ok bool, rest, err string) {
	i := 0
	for i < len(content) && content[i] != ' ' && content[i] != '\t' && content[i] != '(' {
		i++
	}
	head = content[:i]
	if head == "" {
		return "", "", false, "", "empty instruction"
	}
	if i < len(content) && content[i] == '(' {
		ok = true
		depth := 1
		j := i + 1
		for j < len(content) && depth > 0 {
			switch content[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				break
			}
			j++
		}
		if depth != 0 || j >= len(content) {
			return head, "", true, "", "unterminated parameter list, missing ')' "
		}
		params = content[i+1 : j]
		rest = content[j+1:]
		return head, params, true, rest, ""
	}
	rest = content[i:]
	return head, "", false, rest, ""
}

type opToken struct {
	text string
	col  int // 1-based，相对整行
}

type opTokenError struct {
	col int
	msg string
}

// tokenizeOperands 将逗号/空白分隔的操作数切分，并记录每个 token 的列号。
// baseCol 是 content 起始处相对行首的 0-based 偏移。
func tokenizeOperands(rest string, contentOffset int) ([]opToken, *opTokenError) {
	var toks []opToken
	i := 0
	for i < len(rest) {
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == ',') {
			i++
		}
		if i == len(rest) {
			break
		}
		start := i
		for i < len(rest) && rest[i] != ' ' && rest[i] != '\t' && rest[i] != ',' {
			i++
		}
		text := rest[start:i]
		col := contentOffset + start + 1
		if strings.ContainsAny(text, "()") {
			return nil, &opTokenError{col: col, msg: fmt.Sprintf("unexpected token %q (parameters must be attached to the gate name, e.g. rx(90) 0)", text)}
		}
		toks = append(toks, opToken{text: text, col: col})
	}
	return toks, nil
}

// parseParams 解析括号内的逗号分隔浮点参数，个数必须精确等于 want。
func parseParams(raw string, want int) ([]float64, string) {
	parts := strings.Split(raw, ",")
	var out []float64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, "empty parameter value"
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Sprintf("parameter %q is not a valid number (angles are in degrees)", p)
		}
		out = append(out, v)
	}
	if len(out) != want {
		return nil, fmt.Sprintf("expected %d parameter(s), got %d", want, len(out))
	}
	return out, ""
}

func firstNonSpace(s string) int {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

// Validate 校验目标比特范围、参数完整性与声明一致性。
// 校验失败返回类型化错误；调用方不会得到被部分修改的线路。
func (c *Circuit) Validate() error {
	if c.NQubits < 1 {
		return &ConflictError{Line: 1, Message: "qubit count must be at least 1"}
	}
	for _, g := range c.Gates {
		if g.IsMeasure {
			for _, q := range g.Targets {
				if q < 0 || q >= c.NQubits {
					return &TargetOutOfRangeError{
						Op: "measure", Line: g.Line, Target: q, NQubits: c.NQubits,
					}
				}
			}
			continue
		}
		seen := make(map[int]bool)
		for _, q := range g.Operands() {
			if q < 0 || q >= c.NQubits {
				return &TargetOutOfRangeError{
					Op: g.Name, Line: g.Line, Target: q, NQubits: c.NQubits,
				}
			}
			if seen[q] {
				return &DuplicateTargetError{Op: g.Name, Line: g.Line, Target: q}
			}
			seen[q] = true
		}
		if want := ParamCount(g.Name); len(g.Params) != want {
			return &MissingParamError{
				Op: g.Name, Line: g.Line, Param: "theta",
				Message: fmt.Sprintf("expected %d parameter(s), got %d", want, len(g.Params)),
			}
		}
	}
	return nil
}

// NumQubits 返回量子比特数。
func (c *Circuit) NumQubits() int { return c.NQubits }

// GateCount 返回门指令数量（注释/空行不计，展开后的测量各计一条）。
func (c *Circuit) GateCount() int { return len(c.Gates) }
