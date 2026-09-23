package model

import (
	"time"
)

type RequestStatus string

const (
	// RequestAwaitingSubstitute 新申请的第一步：等待替班人确认
	RequestAwaitingSubstitute RequestStatus = "awaiting_substitute"
	// RequestPending 替班人已同意，等待主管审批（历史申请直接处于该状态，仍由主管直接处理）
	RequestPending  RequestStatus = "pending"
	RequestApproved RequestStatus = "approved"
	// RequestRejected 主管驳回
	RequestRejected RequestStatus = "rejected"
	// RequestDeclined 替班人拒绝，申请结束
	RequestDeclined RequestStatus = "declined"
)

type ShiftRequest struct {
	BaseModel
	ApplicantID           uint          `json:"applicant_id"`
	SubstituteID          uint          `json:"substitute_id"`
	ScheduleID            uint          `json:"schedule_id"`
	SubstituteScheduleID  uint          `json:"substitute_schedule_id"`
	Reason                string        `json:"reason"`
	Status                RequestStatus `json:"status"`
	SubstituteConfirmedAt *time.Time    `json:"substitute_confirmed_at"`
	ReviewerID            *uint         `json:"reviewer_id"`
	ReviewedAt            *time.Time    `json:"reviewed_at"`
	Applicant             Staff         `gorm:"foreignKey:ApplicantID" json:"applicant,omitempty"`
	Substitute            Staff         `gorm:"foreignKey:SubstituteID" json:"substitute,omitempty"`
}
