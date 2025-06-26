package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/kerry9118/pbft-chain/api/pb"
	"github.com/kerry9118/pbft-chain/consensus"
	"github.com/kerry9118/pbft-chain/network"
)

func main() {
	nodeID := flag.String("id", "node-0", "目前節點的ID")
	flag.Parse()

	nodeConfigs := map[string]string{
		"node-0": "127.0.0.1:8000",
		"node-1": "127.0.0.1:8001",
		"node-2": "127.0.0.1:8002",
		"node-3": "127.0.0.1:8003",
	}

	selfAddr, ok := nodeConfigs[*nodeID]
	if !ok {
		log.Fatalf("無效的節點ID: %s", *nodeID)
	}
	node := network.NewNode(*nodeID, selfAddr)
	node.Start()
	time.Sleep(1 * time.Second)

	for id, addr := range nodeConfigs {
		if id != *nodeID {
			err := node.Connect(id, addr)
			if err != nil {
				log.Printf("連線到節點 %s 失敗: %v", id, err)
			}
		}
	}

	pbft := consensus.NewPBFTState(node, len(nodeConfigs))
	pbft.StartConsensus()

	if *nodeID == "node-0" {
		log.Println("我是主節點，等待 5 秒後傳送交易...")
		time.Sleep(5 * time.Second)

		tx := &pb.Transaction{
			Id:      fmt.Sprintf("tx-%d", time.Now().Unix()),
			Payload: []byte("這是一筆重要的交易"),
		}
		log.Println("主節點開始處理一筆新交易...")
		pbft.HandleRequest(tx)
	}

	select {}
}
