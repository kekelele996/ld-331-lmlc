package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gbsched/hospital-scheduler/internal/model"
	"github.com/gbsched/hospital-scheduler/internal/repository"
	"gorm.io/gorm"
)

var (
	ErrRequestClosed = errors.New("request already closed")
	ErrNotSubstitute = errors.New("only the designated substitute can respond")
	ErrWrongStage    = errors.New("request is not waiting for this action")
)

type ShiftRequestService struct {
	repo         *repository.ShiftRequestRepository
	scheduleRepo *repository.ScheduleRepository
	db           *gorm.DB
	logger       *slog.Logger
}

func NewShiftRequestService(r *repository.ShiftRequestRepository, s *repository.ScheduleRepository, db *gorm.DB, l *slog.Logger) *ShiftRequestService {
	return &ShiftRequestService{r, s, db, l}
}

func (s *ShiftRequestService) List() ([]model.ShiftRequest, error) {
	v, e := s.repo.List()
	if e != nil {
		return nil, fmt.Errorf("list shift requests: %w", e)
	}
	return v, nil
}

// Create 提交换班申请。新申请先等替班人确认；仅历史申请（改造前已存在的 pending）
// 才直接进入主管审批环节。
func (s *ShiftRequestService) Create(v model.ShiftRequest, actor uint) error {
	a, e := s.scheduleRepo.Find(v.ScheduleID)
	if e != nil {
		return fmt.Errorf("find applicant schedule: %w", e)
	}
	b, e := s.scheduleRepo.Find(v.SubstituteScheduleID)
	if e != nil {
		return fmt.Errorf("find substitute schedule: %w", e)
	}
	if a.StaffID != actor || b.StaffID != v.SubstituteID {
		return fmt.Errorf("schedule ownership does not match applicants")
	}
	v.ApplicantID = actor
	v.Status = model.RequestSubstitutePending
	if e = s.repo.Create(&v); e != nil {
		return fmt.Errorf("create shift request: %w", e)
	}
	return nil
}

// Respond 是替班人对换班申请的表态：同意则流转到主管审批，拒绝则申请直接结束。
func (s *ShiftRequestService) Respond(id, actor uint, approved bool) error {
	r, e := s.repo.Find(id)
	if e != nil {
		return fmt.Errorf("find shift request: %w", e)
	}
	if r.SubstituteID != actor {
		return ErrNotSubstitute
	}
	if r.Status != model.RequestSubstitutePending {
		if isClosed(r.Status) {
			return ErrRequestClosed
		}
		return ErrWrongStage
	}
	now := time.Now()
	r.SubstituteDecidedAt = &now
	if !approved {
		r.Status = model.RequestSubstituteRejected
		if e = s.repo.Save(&r); e != nil {
			return fmt.Errorf("save substitute rejection: %w", e)
		}
		return nil
	}
	r.Status = model.RequestSupervisorPending
	if e = s.repo.Save(&r); e != nil {
		return fmt.Errorf("save substitute approval: %w", e)
	}
	return nil
}

// Review 是主管审批。历史 pending 申请与替班人已同意的申请可直接审批；
// 驳回即结束；通过时在同一事务内交换双方班次，并以条件更新保证重复操作
// 不会把班次再换回来。
func (s *ShiftRequestService) Review(id, reviewer uint, approved bool) error {
	r, e := s.repo.Find(id)
	if e != nil {
		return fmt.Errorf("find shift request: %w", e)
	}
	switch r.Status {
	case model.RequestPending, model.RequestSupervisorPending:
		// 主管可处理：历史申请（pending）或替班人已同意的申请
	default:
		if isClosed(r.Status) {
			return ErrRequestClosed
		}
		return ErrWrongStage
	}
	now := time.Now()
	r.ReviewerID = &reviewer
	r.ReviewedAt = &now
	if !approved {
		r.Status = model.RequestRejected
		if e = s.repo.Save(&r); e != nil {
			return fmt.Errorf("save review: %w", e)
		}
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var a, b model.Schedule
		if e := tx.First(&a, r.ScheduleID).Error; e != nil {
			return fmt.Errorf("load applicant schedule: %w", e)
		}
		if e := tx.First(&b, r.SubstituteScheduleID).Error; e != nil {
			return fmt.Errorf("load substitute schedule: %w", e)
		}
		a.StaffID, b.StaffID = b.StaffID, a.StaffID
		if e := tx.Save(&a).Error; e != nil {
			return e
		}
		if e := tx.Save(&b).Error; e != nil {
			return e
		}
		// 条件更新：状态必须仍停留在当前待审批阶段才允许置为 approved，
		// 防止并发/重复审批触发第二次交换。
		res := tx.Model(&model.ShiftRequest{}).
			Where("id = ? AND status IN ?", id,
				[]model.RequestStatus{model.RequestPending, model.RequestSupervisorPending}).
			Updates(map[string]any{
				"status":        model.RequestApproved,
				"reviewer_id":   reviewer,
				"reviewed_at":   now,
				"updated_at":    time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrRequestClosed
		}
		return nil
	})
}

func isClosed(st model.RequestStatus) bool {
	return st == model.RequestApproved ||
		st == model.RequestRejected ||
		st == model.RequestSubstituteRejected
}
