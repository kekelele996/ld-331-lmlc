package model

import (
	"time"
)

type RequestStatus string

const (
	// RequestPending 为改造前的旧状态：旧申请直接由主管审批，跳过替班人确认。
	RequestPending RequestStatus = "pending"
	// RequestSubstitutePending 等替班人确认；RequestSupervisorPending 替班人已同意，等主管审批。
	RequestSubstitutePending RequestStatus = "substitute_pending"
	RequestSupervisorPending RequestStatus = "supervisor_pending"
	RequestApproved          RequestStatus = "approved"
	// RequestRejected 主管驳回；RequestSubstituteRejected 替班人拒绝，申请直接结束。
	RequestSubstituteRejected RequestStatus = "substitute_rejected"
	RequestRejected           RequestStatus = "rejected"
)

type ShiftRequest struct {
	BaseModel
	ApplicantID           uint          `json:"applicant_id"`
	SubstituteID          uint          `json:"substitute_id"`
	ScheduleID            uint          `json:"schedule_id"`
	SubstituteScheduleID  uint          `json:"substitute_schedule_id"`
	Reason                string        `json:"reason"`
	Status                RequestStatus `json:"status"`
	SubstituteDecidedAt   *time.Time    `json:"substitute_decided_at"`
	ReviewerID            *uint         `json:"reviewer_id"`
	ReviewedAt            *time.Time    `json:"reviewed_at"`
	Applicant             Staff         `gorm:"foreignKey:ApplicantID" json:"applicant,omitempty"`
	Substitute            Staff         `gorm:"foreignKey:SubstituteID" json:"substitute,omitempty"`
}
