package main

import (
	"fmt"
	"strings"
)

type treeNode struct {
	name       string
	isDir      bool
	children   []*treeNode
	tokens     int
	annotation string
	showTokens bool
}

type treeLine struct {
	prefix     string
	tokens     int
	annotation string
	isDir      bool
	showTokens bool
}

func buildTree(filePaths []string, tokenMap map[string]int, modTimeMap map[string]string) *treeNode {
	root := &treeNode{isDir: true}
	for _, path := range filePaths {
		parts := strings.Split(path, "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *treeNode
			for _, c := range current.children {
				if c.name == part {
					child = c
					break
				}
			}
			if child == nil {
				child = &treeNode{
					name:  part,
					isDir: !isLast,
				}
				if isLast {
					child.tokens = tokenMap[path]
					if modTime := modTimeMap[path]; modTime != "" {
						child.annotation = fmt.Sprintf("[mod: %s]", modTime)
					}
					child.showTokens = true
				}
				current.children = append(current.children, child)
			}
			current = child
		}
	}
	aggregateTokens(root)
	return root
}

func buildSinceTree(changes []fileChange, tokenMap map[string]int, statusMap map[string]string) *treeNode {
	root := &treeNode{isDir: true}
	for _, c := range changes {
		parts := strings.Split(c.Path, "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *treeNode
			for _, ch := range current.children {
				if ch.name == part {
					child = ch
					break
				}
			}
			if child == nil {
				child = &treeNode{
					name:  part,
					isDir: !isLast,
				}
				if isLast {
					child.tokens = tokenMap[c.Path]
					status := statusMap[c.Path]
					child.annotation = fmt.Sprintf("[%s]", status)
					child.showTokens = (status != "D")
				}
				current.children = append(current.children, child)
			}
			current = child
		}
	}
	aggregateTokens(root)
	return root
}

func aggregateTokens(node *treeNode) int {
	if !node.isDir {
		return node.tokens
	}
	total := 0
	for _, child := range node.children {
		total += aggregateTokens(child)
	}
	node.tokens = total
	return total
}

func renderTreeLines(root *treeNode) []treeLine {
	var lines []treeLine
	renderChildren(root, "", &lines)
	return lines
}

func renderChildren(node *treeNode, prefix string, lines *[]treeLine) {
	for i, child := range node.children {
		isLast := i == len(node.children)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		name := child.name
		if child.isDir {
			name += "/"
		}

		*lines = append(*lines, treeLine{
			prefix:     prefix + connector + name,
			tokens:     child.tokens,
			annotation: child.annotation,
			isDir:      child.isDir,
			showTokens: child.showTokens,
		})

		if child.isDir {
			childPrefix := prefix + "│   "
			if isLast {
				childPrefix = prefix + "    "
			}
			renderChildren(child, childPrefix, lines)
		}
	}
}

func formatTreeOutput(lines []treeLine) string {
	maxWidth := 0
	for _, l := range lines {
		if w := displayWidth(l.prefix); w > maxWidth {
			maxWidth = w
		}
	}

	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l.prefix)
		padding := maxWidth - displayWidth(l.prefix) + 2
		sb.WriteString(strings.Repeat(" ", padding))

		if l.isDir {
			sb.WriteString(fmt.Sprintf("(~%s tok)", formatNumber(l.tokens)))
		} else if !l.showTokens {
			sb.WriteString(fmt.Sprintf("(deleted)     %s", l.annotation))
		} else {
			sb.WriteString(fmt.Sprintf("(~%s tok)", formatNumber(l.tokens)))
			if l.annotation != "" {
				sb.WriteString(fmt.Sprintf("  %s", l.annotation))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
