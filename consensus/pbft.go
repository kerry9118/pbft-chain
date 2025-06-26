package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"github.com/kerry9118/pbft-chain/api/pb"  // 替換
	"github.com/kerry9118/pbft-chain/network" // 替換
)

// PBFTState 儲存了一個節點在共識過程中的所有狀態
type PBFTState struct {
	Node           *network.Node // 指向節點網路層
	View           int64         // 目前檢視編號
	Sequence       int64         // 目前序號
	PrimaryID      string        // 目前主節點ID
	TotalNodes     int           // 網路中的總節點數
	F              int           // 可容忍的拜占庭節點數 f = (n-1)/3
	State          string        // 節點目前狀態 (e.g., "Idle", "Prepared", "Committed")
	Mutex          sync.RWMutex
	MessageLogs    map[string][]*pb.ConsensusMessage // 儲存收到的訊息, key: digest
	PendingRequest *pb.Transaction                   // 主節點等待處理的請求
}

// NewPBFTState 建立 PBFT 狀態機例項
func NewPBFTState(node *network.Node, totalNodes int) *PBFTState {
	f := (totalNodes - 1) / 3
	return &PBFTState{
		Node:        node,
		View:        0,
		Sequence:    0,
		PrimaryID:   "node-0", // 簡單起見，初始主節點固定
		TotalNodes:  totalNodes,
		F:           f,
		State:       "Idle",
		MessageLogs: make(map[string][]*pb.ConsensusMessage),
	}
}

// StartConsensus 開始處理共識訊息
func (ps *PBFTState) StartConsensus() {
	log.Printf("節點 %s 開始共識流程...", ps.Node.ID)
	// 啟動一個 goroutine 從訊息通道中讀取並處理訊息
	go func() {
		for msg := range ps.Node.MsgChan {
			ps.handleMessage(msg)
		}
	}()
}

// isPrimary 判斷目前節點是否是主節點
func (ps *PBFTState) isPrimary() bool {
	return ps.Node.ID == ps.PrimaryID
}

// HandleRequest 主節點處理來自使用者端的請求
func (ps *PBFTState) HandleRequest(tx *pb.Transaction) {
	if !ps.isPrimary() {
		log.Printf("節點 %s 不是主節點，忽略請求", ps.Node.ID)
		return
	}

	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	// 計算請求摘要
	digest := calculateDigest(tx)

	// 分配序號
	ps.Sequence++

	// 構造 Pre-prepare 訊息
	msg := &pb.ConsensusMessage{
		Type:          pb.ConsensusMessage_PRE_PREPARE,
		View:          ps.View,
		Sequence:      ps.Sequence,
		NodeId:        ps.Node.ID,
		RequestDigest: []byte(digest),
		Request:       []*pb.Transaction{tx}, // 攜帶原始請求
	}

	// 記錄日誌
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)
	log.Printf("[主節點 %s] 傳送 PRE-PREPARE: 檢視 %d, 序號 %d", ps.Node.ID, ps.View, ps.Sequence)

	// 廣播 Pre-prepare 訊息
	ps.Node.Broadcast(msg)
}

// handleMessage 是核心的訊息分發和處理邏輯
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

// calculateDigest 輔助函式，計算交易的雜湊摘要
func calculateDigest(tx *pb.Transaction) string {
	data := tx.Id + string(tx.Payload)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// handlePrePrepare 處理收到的 Pre-prepare 訊息 (備份節點)
func (ps *PBFTState) handlePrePrepare(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("[備份節點 %s] 收到 PRE-PREPARE: 檢視 %d, 序號 %d", ps.Node.ID, msg.View, msg.Sequence)

	// TODO: 這裡應有更嚴格的驗證邏輯
	// 1. 驗證簽名 (簡化專案中忽略)
	// 2. 檢查檢視和序號是否正確
	// 3. 檢查摘要是否與請求內容匹配

	// 記錄日誌
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 構造 Prepare 訊息
	prepareMsg := &pb.ConsensusMessage{
		Type:          pb.ConsensusMessage_PREPARE,
		View:          msg.View,
		Sequence:      msg.Sequence,
		NodeId:        ps.Node.ID,
		RequestDigest: msg.RequestDigest,
	}

	// 記錄自己的 Prepare 訊息
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], prepareMsg)
	log.Printf("[備份節點 %s] 傳送 PREPARE: 檢視 %d, 序號 %d", ps.Node.ID, msg.View, msg.Sequence)

	// 廣播 Prepare 訊息
	ps.Node.Broadcast(prepareMsg)
}

// handlePrepare 處理收到的 Prepare 訊息
func (ps *PBFTState) handlePrepare(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("節點 %s 收到 PREPARE: 來自 %s, 檢視 %d, 序號 %d", ps.Node.ID, msg.NodeId, msg.View, msg.Sequence)

	// 記錄日誌
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 檢查是否收到了足夠的 Prepare 訊息 (2f+1 個，包括 pre-prepare)
	count := 0
	for _, logMsg := range ps.MessageLogs[digest] {
		// PRE-PREPARE 也算一個
		if logMsg.Type == pb.ConsensusMessage_PRE_PREPARE || logMsg.Type == pb.ConsensusMessage_PREPARE {
			count++
		}
	}

	// 2f+1 的閾值，並且目前狀態還不是 Prepared (防止重複傳送 Commit)
	if count >= 2*ps.F+1 && ps.State != "Prepared" {
		ps.State = "Prepared"
		log.Printf("節點 %s 達到 PREPARED 狀態: 檢視 %d, 序號 %d", ps.Node.ID, msg.View, msg.Sequence)

		// 構造 Commit 訊息
		commitMsg := &pb.ConsensusMessage{
			Type:          pb.ConsensusMessage_COMMIT,
			View:          msg.View,
			Sequence:      msg.Sequence,
			NodeId:        ps.Node.ID,
			RequestDigest: msg.RequestDigest,
		}

		// 記錄自己的 Commit 訊息
		ps.MessageLogs[digest] = append(ps.MessageLogs[digest], commitMsg)
		log.Printf("節點 %s 傳送 COMMIT: 檢視 %d, 序號 %d", ps.Node.ID, msg.View, msg.Sequence)

		// 廣播 Commit 訊息
		ps.Node.Broadcast(commitMsg)
	}
}

// handleCommit 處理收到的 Commit 訊息
func (ps *PBFTState) handleCommit(msg *pb.ConsensusMessage) {
	ps.Mutex.Lock()
	defer ps.Mutex.Unlock()

	digest := string(msg.RequestDigest)
	log.Printf("節點 %s 收到 COMMIT: 來自 %s, 檢視 %d, 序號 %d", ps.Node.ID, msg.NodeId, msg.View, msg.Sequence)

	// 記錄日誌
	ps.MessageLogs[digest] = append(ps.MessageLogs[digest], msg)

	// 檢查是否收到了足夠的 Commit 訊息 (2f+1 個)
	count := 0
	for _, logMsg := range ps.MessageLogs[digest] {
		if logMsg.Type == pb.ConsensusMessage_COMMIT {
			count++
		}
	}

	// 達到 2f+1 閾值，並且目前狀態不是 Committed (防止重複提交)
	if count >= 2*ps.F+1 && ps.State != "Committed" {
		ps.State = "Committed"
		log.Printf("✅ 節點 %s 達成共識 (COMMITTED): 檢視 %d, 序號 %d, 摘要 %s...", ps.Node.ID, msg.View, msg.Sequence, digest[:8])

		// TODO: 在這裡執行請求（例如，將交易新增到區塊中）
		// 1. 從訊息日誌中找到原始的 Pre-prepare 訊息，以獲取完整的交易資料
		// 2. 驗證交易
		// 3. 將交易應用到狀態機（例如，更新帳本）
		// 4. 如果是主節點，打包成區塊並廣播

		// 共識完成後，重設狀態以便處理下一個請求
		ps.resetStateAfterCommit()
	}
}

func (ps *PBFTState) resetStateAfterCommit() {
	// 簡單的狀態重設
	ps.State = "Idle"
	// 實際應用中，訊息日誌清理策略會更複雜
}

// 模擬檢視切換（簡化版）
func (ps *PBFTState) StartViewChangeTimer() {
	go func() {
		time.Sleep(5 * time.Second) // 假設5秒沒收到主節點訊息就觸發檢視切換
		ps.Mutex.Lock()
		defer ps.Mutex.Unlock()
		if ps.State != "Committed" && ps.State != "Idle" {
			log.Printf("節點 %s 超時，準備發起檢視切換!", ps.Node.ID)
			// TODO: 實現檢視切換邏輯
			// 1. 廣播 VIEW-CHANGE 訊息
			// 2. 收集 2f+1 個 VIEW-CHANGE 訊息
			// 3. 新主節點廣播 NEW-VIEW 訊息
		}
	}()
}
