package service

import (
	"fmt"
	"github.com/gbsched/hospital-scheduler/internal/model"
	"github.com/gbsched/hospital-scheduler/internal/repository"
	"gorm.io/gorm"
	"log/slog"
	"time"
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
	// 新申请必须先由替班人确认，确认后才进入主管审批队列。
	v.Status = model.RequestAwaitingSubstitute
	if e = s.repo.Create(&v); e != nil {
		return fmt.Errorf("create shift request: %w", e)
	}
	return nil
}

// Confirm 由被指定的替班人表态：同意则流转至主管审批，拒绝则申请直接结束。
func (s *ShiftRequestService) Confirm(id, actor uint, accepted bool) error {
	r, e := s.repo.Find(id)
	if e != nil {
		return fmt.Errorf("find shift request: %w", e)
	}
	if r.SubstituteID != actor {
		return fmt.Errorf("只有被指定的替班人才能确认该申请")
	}
	if r.Status != model.RequestAwaitingSubstitute {
		return fmt.Errorf("该申请当前无需替班人确认，请勿重复操作")
	}
	now := time.Now()
	updates := map[string]any{"substitute_confirmed_at": now, "status": model.RequestPending}
	if !accepted {
		updates["status"] = model.RequestDeclined
	}
	n, e := s.repo.Transition(id, model.RequestAwaitingSubstitute, updates)
	if e != nil {
		return fmt.Errorf("confirm shift request: %w", e)
	}
	if n == 0 {
		return fmt.Errorf("该申请已被处理，请刷新后重试")
	}
	return nil
}
func (s *ShiftRequestService) Review(id, reviewer uint, approved bool) error {
	r, e := s.repo.Find(id)
	if e != nil {
		return fmt.Errorf("find shift request: %w", e)
	}
	if r.Status != model.RequestPending {
		return fmt.Errorf("request already reviewed")
	}
	now := time.Now()
	if !approved {
		n, e := s.repo.Transition(id, model.RequestPending, map[string]any{"status": model.RequestRejected, "reviewer_id": reviewer, "reviewed_at": now})
		if e != nil {
			return fmt.Errorf("reject shift request: %w", e)
		}
		if n == 0 {
			return fmt.Errorf("request already reviewed")
		}
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 条件更新保证重复或并发审批只有一方生效，班次不会被换来换去。
		res := tx.Model(&model.ShiftRequest{}).Where("id = ? AND status = ?", r.ID, model.RequestPending).Updates(map[string]any{"status": model.RequestApproved, "reviewer_id": reviewer, "reviewed_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("request already reviewed")
		}
		var a, b model.Schedule
		if e := tx.First(&a, r.ScheduleID).Error; e != nil {
			return e
		}
		if e := tx.First(&b, r.SubstituteScheduleID).Error; e != nil {
			return e
		}
		a.StaffID, b.StaffID = b.StaffID, a.StaffID
		if e := tx.Save(&a).Error; e != nil {
			return e
		}
		if e := tx.Save(&b).Error; e != nil {
			return e
		}
		return nil
	})
}
