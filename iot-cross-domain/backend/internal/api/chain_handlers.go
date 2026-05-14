package api

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"iot-cross-domain/backend/internal/chain"
	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ChainInfo(c *gin.Context) {
	channel := h.Chain.ChannelName()
	info, infoErr := h.Chain.ChainInfo()

	var lastAnchor model.ChainAnchor
	_ = h.DB.Order("id DESC").Limit(1).First(&lastAnchor).Error

	var totalAnchors int64
	_ = h.DB.Model(&model.ChainAnchor{}).Count(&totalAnchors).Error

	out := gin.H{
		"channel":      channel,
		"chaincode":    "anchorcc",
		"anchorFunc":   "AnchorRecord",
		"totalAnchors": totalAnchors,
		"lastAnchorAt": nilIfZero(lastAnchor.CreatedAt),
		"lastTxHash":   lastAnchor.TxHash,
		"queryOk":      infoErr == nil,
	}
	if infoErr != nil {
		out["queryError"] = infoErr.Error()
		out["height"] = lastAnchor.BlockHeight
	} else {
		out["height"] = info.Height
		out["currentBlockHash"] = info.CurrentBlockHash
		out["previousBlockHash"] = info.PreviousBlockHash
	}
	response.OK(c, out)
}

func (h *Handler) ChainTopology(c *gin.Context) {
	topo := chain.DefaultTopology(h.Chain.ChannelName(), "anchorcc", "AnchorRecord")
	chain.ProbeTopology(&topo)

	online := 0
	total := 0
	for _, org := range topo.Orgs {
		for _, p := range org.Peers {
			total++
			if p.Online {
				online++
			}
		}
	}
	for _, o := range topo.Orderers {
		total++
		if o.Online {
			online++
		}
	}
	response.OK(c, gin.H{
		"topology":    topo,
		"nodesTotal":  total,
		"nodesOnline": online,
	})
}

type chainBlockGroup struct {
	BlockHeight uint64              `json:"blockHeight"`
	TxCount     int                 `json:"txCount"`
	FirstAt     time.Time           `json:"firstAt"`
	LastAt      time.Time           `json:"lastAt"`
	Anchors     []model.ChainAnchor `json:"anchors"`
}

func (h *Handler) ChainBlocks(c *gin.Context) {
	limit := 50
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v <= 200 {
		limit = v
	}

	var anchors []model.ChainAnchor
	if err := h.DB.Order("block_height DESC, id DESC").Limit(limit * 8).Find(&anchors).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	groups := map[uint64]*chainBlockGroup{}
	order := []uint64{}
	for _, a := range anchors {
		g, ok := groups[a.BlockHeight]
		if !ok {
			g = &chainBlockGroup{BlockHeight: a.BlockHeight, FirstAt: a.CreatedAt, LastAt: a.CreatedAt}
			groups[a.BlockHeight] = g
			order = append(order, a.BlockHeight)
		}
		g.Anchors = append(g.Anchors, a)
		g.TxCount++
		if a.CreatedAt.Before(g.FirstAt) {
			g.FirstAt = a.CreatedAt
		}
		if a.CreatedAt.After(g.LastAt) {
			g.LastAt = a.CreatedAt
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] > order[j] })
	if len(order) > limit {
		order = order[:limit]
	}
	out := make([]*chainBlockGroup, 0, len(order))
	for _, bh := range order {
		out = append(out, groups[bh])
	}
	response.OK(c, out)
}

func (h *Handler) ChainBlockDetail(c *gin.Context) {
	num, err := strconv.ParseUint(c.Param("num"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid block number")
		return
	}
	block, queryErr := h.Chain.BlockByNumber(num)

	var anchors []model.ChainAnchor
	_ = h.DB.Where("block_height = ?", num).Order("id ASC").Find(&anchors).Error

	out := gin.H{
		"blockHeight": num,
		"anchors":     anchors,
		"queryOk":     queryErr == nil,
	}
	if queryErr != nil {
		out["queryError"] = queryErr.Error()
	} else {
		out["block"] = block
	}
	response.OK(c, out)
}

func (h *Handler) ChainVerify(c *gin.Context) {
	var req struct {
		TxHash string `json:"txHash"`
		BizRef string `json:"bizRef"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.TxHash == "" && req.BizRef == "" {
		response.BadRequest(c, "txHash or bizRef required")
		return
	}

	q := h.DB.Order("id DESC")
	if req.TxHash != "" {
		q = q.Where("tx_hash = ?", req.TxHash)
	} else {
		q = q.Where("biz_ref = ?", req.BizRef)
	}

	var anchor model.ChainAnchor
	if err := q.First(&anchor).Error; err != nil {
		response.OK(c, gin.H{
			"found":      false,
			"consistent": false,
			"message":    "数据库中未找到对应的链上锚定记录",
		})
		return
	}

	tx, txErr := h.Chain.TransactionByID(anchor.TxHash)
	rec, recErr := h.Chain.QueryAnchor(anchor.BizType, anchor.BizRef)

	digestOnChain := ""
	digestMatch := false
	if recErr == nil && rec != nil {
		digestOnChain = rec.Digest
		digestMatch = rec.Digest == anchor.Digest
	}
	consistent := false
	if txErr == nil && tx != nil && tx.Valid && tx.TxID == anchor.TxHash && digestMatch {
		consistent = true
	}

	out := gin.H{
		"found":         true,
		"anchor":        anchor,
		"consistent":    consistent,
		"chainOk":       txErr == nil,
		"digestMatch":   digestMatch,
		"digestOnChain": digestOnChain,
		"digestInDB":    anchor.Digest,
	}
	if txErr != nil {
		out["chainError"] = txErr.Error()
	} else {
		out["chainTx"] = tx
	}
	if recErr != nil {
		out["queryAnchorError"] = recErr.Error()
	}
	response.OK(c, out)
}

func nilIfZero(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

type hashChainBlock struct {
	Number       uint64 `json:"number"`
	DataHash     string `json:"dataHash"`
	PreviousHash string `json:"previousHash"`
	TxCount      int    `json:"txCount"`
}

func (h *Handler) ChainHashChain(c *gin.Context) {
	limit := 8
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v <= 30 {
		limit = v
	}
	info, err := h.Chain.ChainInfo()
	if err != nil {
		response.OK(c, gin.H{"queryOk": false, "queryError": err.Error(), "blocks": []hashChainBlock{}})
		return
	}
	out := make([]hashChainBlock, 0, limit)
	if info.Height == 0 {
		response.OK(c, gin.H{"queryOk": true, "blocks": out})
		return
	}
	top := info.Height - 1
	for i := 0; i < limit; i++ {
		if top < uint64(i) {
			break
		}
		num := top - uint64(i)
		bs, err := h.Chain.BlockByNumber(num)
		if err != nil {
			break
		}
		out = append(out, hashChainBlock{
			Number:       bs.Number,
			DataHash:     bs.DataHash,
			PreviousHash: bs.PreviousHash,
			TxCount:      bs.TxCount,
		})
	}
	response.OK(c, gin.H{"queryOk": true, "blocks": out})
}

type tamperState struct {
	OriginalDigest string
	TamperedAt     time.Time
}

var tamperedAnchors = map[uint]tamperState{}

func (h *Handler) ChainDemoTamper(c *gin.Context) {
	op := c.GetString("username")
	role := c.GetString("role")
	if role != "ADMIN" {
		response.Forbidden(c, "tamper demo requires ADMIN role")
		return
	}
	var req struct {
		AnchorID uint `json:"anchorId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AnchorID == 0 {
		response.BadRequest(c, "anchorId required")
		return
	}
	var anchor model.ChainAnchor
	if err := h.DB.First(&anchor, req.AnchorID).Error; err != nil {
		// Help users distinguish anchorId (chain_anchors.id) from
		// blockHeight (the number they see in the block list).
		var maxAnchor model.ChainAnchor
		_ = h.DB.Order("id DESC").Limit(1).First(&maxAnchor).Error
		var blockHit model.ChainAnchor
		hint := ""
		if h.DB.Where("block_height = ?", req.AnchorID).Order("id ASC").First(&blockHit).Error == nil {
			hint = fmt.Sprintf("（输入的 %d 看起来是区块号，对应区块包含 anchorId=%d，可尝试该 ID）", req.AnchorID, blockHit.ID)
		} else if maxAnchor.ID > 0 {
			hint = fmt.Sprintf("（当前数据库最大 anchorId 是 %d）", maxAnchor.ID)
		}
		response.BadRequest(c, fmt.Sprintf("anchorId=%d 不存在%s", req.AnchorID, hint))
		return
	}
	if _, ok := tamperedAnchors[anchor.ID]; ok {
		response.OK(c, gin.H{"tampered": true, "message": "已处于篡改状态，可直接调用 verify 观察对比"})
		return
	}
	tamperedAnchors[anchor.ID] = tamperState{OriginalDigest: anchor.Digest, TamperedAt: time.Now()}
	tamperedDigest := "tampered_" + anchor.Digest[:8] + "_demo_tampered"
	if err := h.DB.Model(&anchor).Update("digest", tamperedDigest).Error; err != nil {
		delete(tamperedAnchors, anchor.ID)
		response.InternalError(c, err.Error())
		return
	}
	_ = h.auditEx(model.AuditLog{
		Module: "chain", Action: "tamper_demo", Operator: op, Result: "OK",
		Message: fmt.Sprintf("anchor#%d digest tampered for demo", anchor.ID),
	})
	response.OK(c, gin.H{
		"tampered":         true,
		"anchorId":         anchor.ID,
		"txHash":           anchor.TxHash,
		"originalDigest":   tamperedAnchors[anchor.ID].OriginalDigest,
		"tamperedDigest":   tamperedDigest,
		"hint":             "现在请到链上读验证面板，对此 TxHash 触发 verify，可观察到链上记录与数据库不一致",
	})
}

func (h *Handler) ChainDemoRestore(c *gin.Context) {
	op := c.GetString("username")
	role := c.GetString("role")
	if role != "ADMIN" {
		response.Forbidden(c, "tamper demo requires ADMIN role")
		return
	}
	var req struct {
		AnchorID uint `json:"anchorId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AnchorID == 0 {
		response.BadRequest(c, "anchorId required")
		return
	}
	state, ok := tamperedAnchors[req.AnchorID]
	if !ok {
		response.BadRequest(c, "anchor was never tampered in this session")
		return
	}
	if err := h.DB.Model(&model.ChainAnchor{}).Where("id = ?", req.AnchorID).Update("digest", state.OriginalDigest).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	delete(tamperedAnchors, req.AnchorID)
	_ = h.auditEx(model.AuditLog{
		Module: "chain", Action: "tamper_restore", Operator: op, Result: "OK",
		Message: fmt.Sprintf("anchor#%d digest restored", req.AnchorID),
	})
	response.OK(c, gin.H{"restored": true, "anchorId": req.AnchorID})
}

func (h *Handler) ChainDemoStatus(c *gin.Context) {
	out := []gin.H{}
	for id, st := range tamperedAnchors {
		out = append(out, gin.H{"anchorId": id, "originalDigest": st.OriginalDigest, "tamperedAt": st.TamperedAt})
	}
	response.OK(c, gin.H{"tamperedCount": len(tamperedAnchors), "items": out})
}
