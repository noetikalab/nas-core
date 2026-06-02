package handler

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"time"

	"nas/repository"

	"github.com/gin-gonic/gin"
)

// GetProof 查询指定日志的存证记录（操作详情 + 哈希链位置 + 签名状态）。
//
// @Summary      Get proof record for a log entry
// @Description  Return the certified operation detail and its proof chain record.
// @Tags         proof
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Log ID"
// @Success      200 {object} ProofDetailResponse "Proof detail"
// @Failure      400 {object} ErrorResponse "Invalid ID"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      404 {object} ErrorResponse "Not found"
// @Router       /api/proof/{id} [get]
func GetProof(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	op, err := certifiedRepo().GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	proof, _ := proofRepo().GetByCertID(c.Request.Context(), id)

	resp := ProofDetailResponse{
		Operation: toCertifiedOperation(op),
	}
	if proof != nil {
		resp.ProofRecord = toProofRecordResponse(proof)
	}

	c.JSON(http.StatusOK, resp)
}

// GetProofBundle 导出指定范围的完整存证包（操作记录 + 哈希链 + 设备 UID）。
// 验证方拿到这个 JSON 包后用 PUF 公钥即可独立验证。
//
// @Summary      Export proof bundle for verification
// @Description  Return a JSON bundle containing operations, hash chain, and device UID.
// @Tags         proof
// @Produce      json
// @Security     BearerAuth
// @Param        from query int false "起始链序号" default(1)
// @Param        to   query int false "截止链序号"
// @Success      200 {object} ProofBundle "Exported proof bundle"
// @Failure      400 {object} ErrorResponse "Invalid range"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Router       /api/proof/bundle [get]
func GetProofBundle(c *gin.Context) {
	from, _ := strconv.Atoi(c.DefaultQuery("from", "1"))
	to, _ := strconv.Atoi(c.Query("to"))
	if to == 0 {
		idx, _ := proofRepo().NextChainIndex(c.Request.Context())
		to = idx - 1
	}
	if from < 1 || to < from {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid range"})
		return
	}

	rows, err := proofRepo().QueryRange(c.Request.Context(), from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "proof query failed"})
		return
	}

	records := make([]ProofRecordResponse, len(rows))
	ops := make([]CertifiedOperation, len(rows))
	for i, r := range rows {
		records[i] = toProofRecordResponse(&r)
		// 查找对应的操作记录
		if op, err := certifiedRepo().GetByID(c.Request.Context(), r.CertID); err == nil {
			ops[i] = toCertifiedOperation(op)
		}
	}

	c.JSON(http.StatusOK, ProofBundle{
		Records:    records,
		Operations: ops,
		ExportTime: time.Now().UnixNano(),
		TotalCount: len(records),
	})
}

// toProofRecordResponse 将数据库行转换为 API 响应 DTO。
func toProofRecordResponse(r *repository.ProofRow) ProofRecordResponse {
	return ProofRecordResponse{
		CertID:       r.CertID,
		ChainIndex:   r.ChainIndex,
		PrevHash:     base64.StdEncoding.EncodeToString(r.PrevHash),
		DataHash:     base64.StdEncoding.EncodeToString(r.DataHash),
		Signature:    base64.StdEncoding.EncodeToString(r.Signature),
		DeviceUID:    base64.StdEncoding.EncodeToString(r.DeviceUID),
		SigTimestamp: r.SigTimestamp,
		HashAlgo:     r.HashAlgo,
	}
}

