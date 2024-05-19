package main

import (
	"strings"

	"github.com/jaketrock/zome/sync/raft"
)

func GetLocalNode(nodes []raft.Node) raft.Node {
	// The very first node is the local node (i.e. this server).
	return nodes[0]
}
func GetLocalPort(nodes []raft.Node) string {
	// The very first node is the local port value.
	return ":" + nodes[0].Port
}

// Returns other nodes in the cluster besides this one.
func GetOtherNodes(nodes []raft.Node) []raft.Node {
	result := append([]raft.Node(nil), nodes...)
	// Delete first element.
	result = append(result[:0], result[1:]...)
	return result
}

func ParseNodes(input string) []raft.Node {
	pieces := strings.Split(input, ",")
	result := make([]raft.Node, 0)
	for _, nodeString := range pieces {
		result = append(result, ParseNodePortPairString(nodeString))
	}
	return result
}

func ParseNodePortPairString(input string) raft.Node {
	pieces := strings.Split(input, ":")
	return raft.Node{Hostname: pieces[0], Port: pieces[1]}
}
