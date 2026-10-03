package qsim

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DrawConfig 控制线路图渲染选项。
type DrawConfig struct {
	ShowStepNumbers bool // 在每列上方显示门序号（从 1 开始）
}

// DefaultDrawConfig 返回默认渲染配置。
func DefaultDrawConfig() DrawConfig { return DrawConfig{ShowStepNumbers: true} }

// RenderASCII 返回线路的 ASCII 图字符串。
// 每条量子比特一根水平线，门按声明顺序从左到右排列：
//
//	q0: --[H]--*--------
//	q1: -------*--[X]---
func RenderASCII(c *Circuit, cfg DrawConfig) (string, error) {
	var sb strings.Builder
	if err := DrawASCII(c, &sb, cfg); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// DrawASCII 将线路渲染为 ASCII 文本形式，写入 w。
func DrawASCII(c *Circuit, w io.Writer, cfg DrawConfig) error {
	if c == nil {
		return fmt.Errorf("nil circuit")
	}
	n := c.NQubits

	// 每列宽度由该门的目标标签决定。
	widths := make([]int, len(c.Gates))
	cells := make([][]string, len(c.Gates)) // cells[col][row]
	for ci, g := range c.Gates {
		label := gateCellLabel(g)
		widths[ci] = len(label)
		col := make([]string, n)
		minQ, maxQ := n, -1
		role := make(map[int]string)
		for _, q := range g.Controls {
			role[q] = "ctl"
			if q < minQ {
				minQ = q
			}
			if q > maxQ {
				maxQ = q
			}
		}
		for _, q := range g.Targets {
			if g.Name == "SWAP" {
				role[q] = "swap"
			} else {
				role[q] = "tgt"
			}
			if q < minQ {
				minQ = q
			}
			if q > maxQ {
				maxQ = q
			}
		}
		W := widths[ci]
		for q := 0; q < n; q++ {
			switch role[q] {
			case "tgt":
				col[q] = label
			case "ctl":
				col[q] = centerMarker("*", W, "-")
			case "swap":
				col[q] = centerMarker("x", W, "-")
			default:
				if q > minQ && q < maxQ {
					col[q] = centerMarker("|", W, "-")
				} else {
					col[q] = strings.Repeat("-", W)
				}
			}
		}
		cells[ci] = col
	}

	prefixWidth := len(fmt.Sprintf("q%d: ", n-1))

	if cfg.ShowStepNumbers && len(c.Gates) > 0 {
		parts := make([]string, len(c.Gates))
		for ci := range c.Gates {
			num := strconv.Itoa(ci + 1)
			parts[ci] = centerMarker(num, widths[ci], " ")
		}
		fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", prefixWidth), strings.Join(parts, " "))
	}

	for q := 0; q < n; q++ {
		parts := make([]string, len(c.Gates))
		for ci := range c.Gates {
			parts[ci] = cells[ci][q]
		}
		wire := ""
		if len(parts) == 0 {
			wire = "--"
		} else {
			wire = "-" + strings.Join(parts, "-") + "-"
		}
		fmt.Fprintf(w, "q%d: %s\n", q, wire)
	}
	return nil
}

// gateCellLabel 返回目标比特所在单元格的完整标签（含括号）。
func gateCellLabel(g *Gate) string {
	if g.IsMeasure {
		return "[M]"
	}
	name := g.Name
	if len(g.Params) > 0 {
		name = fmt.Sprintf("%s(%s)", name, formatParam(g.Params[0]))
	}
	return "[" + name + "]"
}

func formatParam(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// centerMarker 将（长度可能大于 1 的）标记在宽度 W 内居中，两侧用 filler 补齐。
func centerMarker(marker string, W int, filler string) string {
	if len(marker) >= W {
		return marker
	}
	total := W - len(marker)
	left := total / 2
	right := total - left
	return strings.Repeat(filler, left) + marker + strings.Repeat(filler, right)
}
