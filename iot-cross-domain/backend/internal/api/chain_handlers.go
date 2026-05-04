package api

import (
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
	consistent := false
	if txErr == nil && tx != nil {
		consistent = tx.Valid && tx.TxID == anchor.TxHash
	}

	out := gin.H{
		"found":      true,
		"anchor":     anchor,
		"consistent": consistent,
		"chainOk":    txErr == nil,
	}
	if txErr != nil {
		out["chainError"] = txErr.Error()
	} else {
		out["chainTx"] = tx
	}
	response.OK(c, out)
}

func nilIfZero(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
