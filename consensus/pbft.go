package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"github.com/kerry9118/pbft-chain/api/pb"
	"github.com/kerry9118/pbft-chain/network"
)

// PBFTState 存储了一个节点在共识过程中的所有状态
type PBFTState struct {
	Node           *network.Node // 指向节点网络层
	View           int64         // 当前视图编号
	Sequence       int64         // 当前序列号
	PrimaryID      string        // 当前主节点ID
	TotalNodes     int           // 网络中的总节点数
	F              int           // 可容忍的拜占庭节点数 f = (n-1)/3
	State          string        // 节点当前状态 (e.g., "Idle", "Prepared", "Committed")
	Mutex          sync.RWMutex
	MessageLogs    map[string][]*pb.ConsensusMessage // 存储收到的消息, key: digest
	PendingRequest *pb.Transaction                   // 主节点等待处理的请求
}

// NewPBFTState 创建 PBFT 状态机实例
func NewPBFTState(node *network.Node, totalNodes int) *PBFTState {
	f := (totalNodes - 1) / 3
	return &PBFTState{
		Node:        node,
		View:        0,
		Sequence:    0,
		PrimaryID:   "node-0", // 简单起见，初始主节点固定
		TotalNodes:  totalNodes,
		F:           f,
		State:       "Idle",
		MessageLogs: make(map[string][]*pb.ConsensusMessage),
	}
}

// StartConsensus 开始处理共识消息
func (ps *PBFTState) StartConsensus() {
	log.Printf("节点 %s 开始共识流程...", ps.Node.ID)
	// 启动一个 goroutine 从消息通道中读取并处理消息
	go func() {
		for msg := range ps.Node.MsgChan {
			ps.handleMessage(msg)
		}
	}()
}

// isPrimary 判断当前节点是否是主节点
func (ps *PBFTState) isPrimary() bool {
	return ps.Node.ID == ps.PrimaryID
}

// HandleRequest 主节点处理来自客户端的请求
func (ps *PBFTState) HandleRequest(tx *pb.Transaction) {
	if !ps.isPrimary() {
		log.Printf("节点 %s 不是主节点，忽略请求", ps.Node.ID)
		return
	}

	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	// 计算请求摘要
	digest := calculateDigest(tx)

	// 分配序列号
	ps.Sequence++

	// 构造 Pre-prepare 消息
	msg := &pb.ConsensusMessage{
		Type:          pb.ConsensusMessage_PRE_PREPARE,
		View:          ps.View,
		Sequence:      ps.Sequence,
		NodeId:        ps.Node.ID,
		RequestDigest: []byte(digest),
		Request:       []*pb.Transaction{tx}, // 携带原始请求
	}

	// 记录日志
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)
	log.Printf("[主节点 %s] 发送 PRE-PREPARE: 视图 %d, 序号 %d", ps.Node.ID, ps.View, ps.Sequence)

	// 广播 Pre-prepare 消息
	ps.Node.Broadcast(msg)
}

// handleMessage 是核心的消息分发和处理逻辑
func (ps *PBFTState) handleMessage(msg *pb.ConsensusMessage) {
	switch msg.Type {
	case pb.ConsensusMessage_PRE_PREPARE:
		ps.handlePrePrepare(msg)
	case pb.ConsensusMessage_PREPARE:
		ps.handlePrepare(msg)
	case pb.ConsensusMessage_COMMIT:
		ps.handleCommit(msg)
	}
}

// calculateDigest 辅助函数，计算交易的哈希摘要
func calculateDigest(tx *pb.Transaction) string {
	data := tx.Id + string(tx.Payload)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// handlePrePrepare 处理收到的 Pre-prepare 消息 (备份节点)
func (ps *PBFTState) handlePrePrepare(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("[备份节点 %s] 收到 PRE-PREPARE: 视图 %d, 序号 %d", ps.Node.ID, msg.View, msg.Sequence)

	// TODO: 这里应有更严格的验证逻辑
	// 1. 验证签名 (简化项目中忽略)
	// 2. 检查视图和序列号是否正确
	// 3. 检查摘要是否与请求内容匹配

	// 记录日志
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 构造 Prepare 消息
	prepareMsg := &pb.ConsensusMessage{
		Type:          pb.ConsensusMessage_PREPARE,
		View:          msg.View,
		Sequence:      msg.Sequence,
		NodeId:        ps.Node.ID,
		RequestDigest: msg.RequestDigest,
	}

	// 记录自己的 Prepare 消息
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], prepareMsg)
	log.Printf("[备份节点 %s] 发送 PREPARE: 视图 %d, 序号 %d", ps.Node.ID, msg.View, msg.Sequence)

	// 广播 Prepare 消息
	ps.Node.Broadcast(prepareMsg)
}

// handlePrepare 处理收到的 Prepare 消息
func (ps *PBFTState) handlePrepare(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("节点 %s 收到 PREPARE: 来自 %s, 视图 %d, 序号 %d", ps.Node.ID, msg.NodeId, msg.View, msg.Sequence)

	// 记录日志
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 检查是否收到了足够的 Prepare 消息 (2f+1 个，包括 pre-prepare)
	count := 0
	for _, logMsg := range ps.MessageLogs[digest] {
		// PRE-PREPARE 也算一个
		if logMsg.Type == pb.ConsensusMessage_PRE_PREPARE || logMsg.Type == pb.ConsensusMessage_PREPARE {
			count++
		}
	}

	// 2f+1 的阈值，并且当前状态还不是 Prepared (防止重复发送 Commit)
	if count >= 2*ps.F+1 && ps.State != "Prepared" {
		ps.State = "Prepared"
		log.Printf("节点 %s 达到 PREPARED 状态: 视图 %d, 序号 %d", ps.Node.ID, msg.View, msg.Sequence)

		// 构造 Commit 消息
		commitMsg := &pb.ConsensusMessage{
			Type:          pb.ConsensusMessage_COMMIT,
			View:          msg.View,
			Sequence:      msg.Sequence,
			NodeId:        ps.Node.ID,
			RequestDigest: msg.RequestDigest,
		}

		// 记录自己的 Commit 消息
		ps.MessageLogs[digest] = append(ps.MessageLogs[digest], commitMsg)
		log.Printf("节点 %s 发送 COMMIT: 视图 %d, 序号 %d", ps.Node.ID, msg.View, msg.Sequence)

		// 广播 Commit 消息
		ps.Node.Broadcast(commitMsg)
	}
}

// handleCommit 处理收到的 Commit 消息
func (ps *PBFTState) handleCommit(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("节点 %s 收到 COMMIT: 来自 %s, 视图 %d, 序号 %d", ps.Node.ID, msg.NodeId, msg.View, msg.Sequence)

	// 记录日志
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 检查是否收到了足够的 Commit 消息 (2f+1 个)
	count := 0
	for _, logMsg := range ps.MessageLogs[digest] {
		if logMsg.Type == pb.ConsensusMessage_COMMIT {
			count++
		}
	}

	// 达到 2f+1 阈值，并且当前状态不是 Committed (防止重复提交)
	if count >= 2*ps.F+1 && ps.State != "Committed" {
		ps.State = "Committed"
		log.Printf("✅ 节点 %s 达成共识 (COMMITTED): 视图 %d, 序号 %d, 摘要 %s...", ps.Node.ID, msg.View, msg.Sequence, digest[:8])

		// TODO: 在这里执行请求（例如，将交易添加到区块中）
		// 1. 从消息日志中找到原始的 Pre-prepare 消息，以获取完整的交易数据
		// 2. 验证交易
		// 3. 将交易应用到状态机（例如，更新账本）
		// 4. 如果是主节点，打包成区块并广播

		// 共识完成后，重置状态以便处理下一个请求
		ps.resetStateAfterCommit()
	}
}

func (ps *PBFTState) resetStateAfterCommit() {
	// 简单的状态重置
	ps.State = "Idle"
	// 实际应用中，消息日志清理策略会更复杂
}

// 模拟视图切换（简化版）
func (ps *PBFTState) StartViewChangeTimer() {
	go func() {
		time.Sleep(5 * time.Second) // 假设5秒没收到主节点消息就触发视图切换
		ps.Mutex.Lock()
		defer ps.Mutex.Unlock()
		if ps.State != "Committed" && ps.State != "Idle" {
			log.Printf("节点 %s 超时，准备发起视图切换!", ps.Node.ID)
			// TODO: 实现视图切换逻辑
			// 1. 广播 VIEW-CHANGE 消息
			// 2. 收集 2f+1 个 VIEW-CHANGE 消息
			// 3. 新主节点广播 NEW-VIEW 消息
		}
	}()
}
