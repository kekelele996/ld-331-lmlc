package handler

import (
	"errors"
	"net/http"

	"github.com/gbsched/hospital-scheduler/internal/constants"
	"github.com/gbsched/hospital-scheduler/internal/dto"
	"github.com/gbsched/hospital-scheduler/internal/middleware"
	"github.com/gbsched/hospital-scheduler/internal/model"
	"github.com/gbsched/hospital-scheduler/internal/service"
	"github.com/gbsched/hospital-scheduler/pkg/response"
	"github.com/gin-gonic/gin"
)

type ShiftRequestHandler struct{ s *service.ShiftRequestService }

func NewShiftRequestHandler(s *service.ShiftRequestService) *ShiftRequestHandler {
	return &ShiftRequestHandler{s}
}
func (h *ShiftRequestHandler) List(c *gin.Context) {
	v, e := h.s.List()
	if e != nil {
		handleError(c, e)
		return
	}
	response.OK(c, v)
}
func (h *ShiftRequestHandler) Create(c *gin.Context) {
	var q dto.ShiftRequestCreate
	if e := c.ShouldBindJSON(&q); e != nil {
		response.Error(c, 400, constants.CodeBadRequest, "调班申请参数错误")
		return
	}
	if e := h.s.Create(model.ShiftRequest{ScheduleID: q.ScheduleID, SubstituteID: q.SubstituteID, SubstituteScheduleID: q.SubstituteScheduleID, Reason: q.Reason}, middleware.StaffID(c)); e != nil {
		handleError(c, e)
		return
	}
	response.OK(c, "申请已提交，等待替班人确认")
}

// Respond 替班人同意/拒绝换班申请
func (h *ShiftRequestHandler) Respond(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var q dto.ReviewRequest
	if e := c.ShouldBindJSON(&q); e != nil {
		response.Error(c, 400, constants.CodeBadRequest, "确认参数错误")
		return
	}
	if e := h.s.Respond(id, middleware.StaffID(c), q.Approved); e != nil {
		h.handleActionError(c, e)
		return
	}
	if q.Approved {
		response.OK(c, "已同意，等待主管审批")
	} else {
		response.OK(c, "已拒绝，申请结束")
	}
}

func (h *ShiftRequestHandler) Review(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var q dto.ReviewRequest
	if e := c.ShouldBindJSON(&q); e != nil {
		response.Error(c, 400, constants.CodeBadRequest, "审批参数错误")
		return
	}
	if e := h.s.Review(id, middleware.StaffID(c), q.Approved); e != nil {
		h.handleActionError(c, e)
		return
	}
	response.OK(c, "审批完成")
}

func (h *ShiftRequestHandler) handleActionError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, service.ErrNotSubstitute):
		response.Error(c, http.StatusForbidden, constants.CodeForbidden, "只有被指定的替班人可以表态")
	case errors.Is(e, service.ErrRequestClosed):
		response.Error(c, http.StatusConflict, constants.CodeBadRequest, "申请已结束，不能重复操作")
	case errors.Is(e, service.ErrWrongStage):
		response.Error(c, http.StatusConflict, constants.CodeBadRequest, "当前阶段不能执行该操作")
	default:
		handleError(c, e)
	}
}
