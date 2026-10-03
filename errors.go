package qsim

import "fmt"

// ParseError 表示线路描述文本中无法解析的指令。
// Line/Column 从 1 开始，用于精确定位出错位置。
type ParseError struct {
	Line    int
	Column  int
	Op      string
	Source  string
	Message string
}

func (e *ParseError) Error() string {
	loc := fmt.Sprintf("line %d", e.Line)
	if e.Column > 0 {
		loc = fmt.Sprintf("line %d, column %d", e.Line, e.Column)
	}
	if e.Op != "" {
		return fmt.Sprintf("parse error at %s near %q: %s", loc, e.Op, e.Message)
	}
	return fmt.Sprintf("parse error at %s: %s", loc, e.Message)
}

// TargetOutOfRangeError 表示门作用的目标比特超出 [0, n) 范围。
type TargetOutOfRangeError struct {
	Op      string
	Line    int
	Target  int
	NQubits int
}

func (e *TargetOutOfRangeError) Error() string {
	return fmt.Sprintf("target out of range at line %d: gate %s targets qubit %d, "+
		"but circuit declares %d qubits (valid range 0..%d)",
		e.Line, e.Op, e.Target, e.NQubits, e.NQubits-1)
}

// MissingParamError 表示参数化门缺少必需参数或参数无法解析。
type MissingParamError struct {
	Op      string
	Line    int
	Param   string
	Message string
}

func (e *MissingParamError) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("missing/invalid parameter %q for gate %s at line %d: %s",
			e.Param, e.Op, e.Line, e.Message)
	}
	return fmt.Sprintf("missing/invalid parameter for gate %s at line %d: %s",
		e.Op, e.Line, e.Message)
}

// ConflictError 表示线路中存在互相冲突的声明
// （如重复 qubit 声明、未声明 qubit 数即使用门等）。
type ConflictError struct {
	Line    int
	Column  int
	Op      string
	Message string
}

func (e *ConflictError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("conflicting declaration at line %d near %q: %s",
			e.Line, e.Op, e.Message)
	}
	return fmt.Sprintf("conflicting declaration at line %d: %s", e.Line, e.Message)
}

// DuplicateTargetError 表示同一条门指令内目标比特重复。
type DuplicateTargetError struct {
	Op     string
	Line   int
	Target int
}

func (e *DuplicateTargetError) Error() string {
	return fmt.Sprintf("duplicate target at line %d: gate %s lists qubit %d more than once",
		e.Line, e.Op, e.Target)
}
