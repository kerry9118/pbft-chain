package network

import (
	"context"
	"log"
	"net"

	"github.com/kerry9118/pbft-chain/api/pb" // 替換
	"google.golang.org/grpc"
)

// NodeServiceServer 實現了 .proto 檔案中定義的服務
type NodeServiceServer struct {
	pb.UnimplementedNodeServiceServer
	Node *Node // 指向擁有該服務的節點
}

// Broadcast 是 gRPC 服務的實現，當節點收到廣播時呼叫此方法
func (s *NodeServiceServer) Broadcast(ctx context.Context, msg *pb.ConsensusMessage) (*pb.ConsensusMessage, error) {
	// 將收到的訊息放入節點的 channel 中，由共識層處理
	s.Node.MsgChan <- msg
	return &pb.ConsensusMessage{}, nil // 簡單返回，實際可以返回確認訊息
}

// startServer 是一個內部方法，用於啟動 gRPC 伺服器
func (n *Node) startServer() {
	lis, err := net.Listen("tcp", n.Addr)
	if err != nil {
		log.Fatalf("節點 %s 監聽失敗: %v", n.ID, err)
	}

	s := grpc.NewServer()
	n.Server = s // 儲存 server 例項，方便後續優雅關閉

	// 註冊服務
	pb.RegisterNodeServiceServer(s, &NodeServiceServer{Node: n})

	if err := s.Serve(lis); err != nil {
		log.Fatalf("節點 %s 啟動 gRPC 服務失敗: %v", n.ID, err)
	}
}
