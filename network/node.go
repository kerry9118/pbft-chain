package network

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/kerry9118/pbft-chain/api/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Node 代表网络中的一个节点
type Node struct {
	ID      string                          // 节点ID
	Addr    string                          // 节点地址 ip:port
	Peers   map[string]pb.NodeServiceClient // 连接的其他节点
	Server  *grpc.Server
	Mutex   sync.RWMutex
	MsgChan chan *pb.ConsensusMessage // 用于接收共识消息
}

// NewNode 创建一个新节点实例
func NewNode(id, addr string) *Node {
	return &Node{
		ID:      id,
		Addr:    addr,
		Peers:   make(map[string]pb.NodeServiceClient),
		MsgChan: make(chan *pb.ConsensusMessage, 100), // 带缓冲的channel
	}
}

// Start 启动 gRPC 服务器并监听
func (n *Node) Start() {
	// ... 服务器启动逻辑将在 server.go 中实现
	log.Printf("节点 %s 在 %s 启动 gRPC 服务", n.ID, n.Addr)
	go n.startServer()
}

// Connect 连接到其他节点
func (n *Node) Connect(peerID, peerAddr string) error {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	if _, ok := n.Peers[peerID]; ok {
		return nil // 已经连接
	}

	conn, err := grpc.Dial(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("无法连接到节点 %s: %v", peerAddr, err)
	}

	client := pb.NewNodeServiceClient(conn)
	n.Peers[peerID] = client
	log.Printf("节点 %s 成功连接到 %s (%s)", n.ID, peerID, peerAddr)
	return nil
}

// Broadcast 向所有已连接的对等节点广播消息
func (n *Node) Broadcast(msg *pb.ConsensusMessage) {
	n.Mutex.RLock()
	defer n.Mutex.RUnlock()

	for peerID, client := range n.Peers {
		go func(pid string, c pb.NodeServiceClient) {
			_, err := c.Broadcast(context.Background(), msg)
			if err != nil {
				log.Printf("节点 %s 向 %s 广播消息失败: %v", n.ID, pid, err)
			}
		}(peerID, client)
	}
}
