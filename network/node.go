package network

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/kerry9118/pbft-chain/api/pb" // 替換為你的專案路徑
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Node 代表網路中的一個節點
type Node struct {
	ID      string                          // 節點ID
	Addr    string                          // 節點位址 ip:port
	Peers   map[string]pb.NodeServiceClient // 連線的其他節點
	Server  *grpc.Server
	Mutex   sync.RWMutex
	MsgChan chan *pb.ConsensusMessage // 用於接收共識訊息
}

// NewNode 建立一個新節點例項
func NewNode(id, addr string) *Node {
	return &Node{
		ID:      id,
		Addr:    addr,
		Peers:   make(map[string]pb.NodeServiceClient),
		MsgChan: make(chan *pb.ConsensusMessage, 100), // 帶緩衝的channel
	}
}

// Start 啟動 gRPC 伺服器並監聽
func (n *Node) Start() {
	// ... 伺服器啟動邏輯將在 server.go 中實現
	log.Printf("節點 %s 在 %s 啟動 gRPC 服務", n.ID, n.Addr)
	go n.startServer()
}

// Connect 連線到其他節點
func (n *Node) Connect(peerID, peerAddr string) error {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	if _, ok := n.Peers[peerID]; ok {
		return nil // 已經連線
	}

	conn, err := grpc.Dial(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("無法連線到節點 %s: %v", peerAddr, err)
	}

	client := pb.NewNodeServiceClient(conn)
	n.Peers[peerID] = client
	log.Printf("節點 %s 成功連線到 %s (%s)", n.ID, peerID, peerAddr)
	return nil
}

// Broadcast 向所有已連線的對等節點廣播訊息
func (n *Node) Broadcast(msg *pb.ConsensusMessage) {
	n.Mutex.RLock()
	defer n.Mutex.RUnlock()

	for peerID, client := range n.Peers {
		go func(pid string, c pb.NodeServiceClient) {
			_, err := c.Broadcast(context.Background(), msg)
			if err != nil {
				log.Printf("節點 %s 向 %s 廣播訊息失敗: %v", n.ID, pid, err)
			}
		}(peerID, client)
	}
}
