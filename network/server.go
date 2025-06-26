package network

import (
	"context"
	"log"
	"net"

	"github.com/kerry9118/pbft-chain/api/pb" // 替换
	"google.golang.org/grpc"
)

// NodeServiceServer 实现了 .proto 文件中定义的服务
type NodeServiceServer struct {
	pb.UnimplementedNodeServiceServer
	Node *Node // 指向拥有该服务的节点
}

// Broadcast 是 gRPC 服务的实现，当节点收到广播时调用此方法
func (s *NodeServiceServer) Broadcast(ctx context.Context, msg *pb.ConsensusMessage) (*pb.ConsensusMessage, error) {
	// 将收到的消息放入节点的 channel 中，由共识层处理
	s.Node.MsgChan <- msg
	// log.Printf("节点 %s 收到来自 %s 的广播消息: 类型 %s, 视图 %d, 序号 %d", s.Node.ID, msg.NodeId, msg.Type, msg.View, msg.Sequence)
	return &pb.ConsensusMessage{}, nil // 简单返回，实际可以返回确认信息
}

// startServer 是一个内部方法，用于启动 gRPC 服务器
func (n *Node) startServer() {
	lis, err := net.Listen("tcp", n.Addr)
	if err != nil {
		log.Fatalf("节点 %s 监听失败: %v", n.ID, err)
	}

	s := grpc.NewServer()
	n.Server = s // 保存 server 实例，方便后续优雅关闭

	// 注册服务
	pb.RegisterNodeServiceServer(s, &NodeServiceServer{Node: n})

	if err := s.Serve(lis); err != nil {
		log.Fatalf("节点 %s 启动 gRPC 服务失败: %v", n.ID, err)
	}
}
