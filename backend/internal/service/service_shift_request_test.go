package service

import (
	"github.com/gbsched/hospital-scheduler/internal/model"
	"github.com/gbsched/hospital-scheduler/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"testing"
	"time"
)

func newShiftRequestService(t *testing.T) (*ShiftRequestService, *gorm.DB) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&model.Staff{}, &model.Shift{}, &model.Schedule{}, &model.ShiftRequest{}); e != nil {
		t.Fatal(e)
	}
	db.Create(&model.Staff{Name: "申请人", Username: "applicant", Active: true, DepartmentID: 1})
	db.Create(&model.Staff{Name: "替班人", Username: "substitute", Active: true, DepartmentID: 1})
	db.Create(&model.Staff{Name: "主管", Username: "supervisor", Active: true, DepartmentID: 1})
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	db.Create(&model.Schedule{DepartmentID: 1, StaffID: 1, ShiftID: 1, WorkDate: day})
	db.Create(&model.Schedule{DepartmentID: 1, StaffID: 2, ShiftID: 2, WorkDate: day})
	s := NewShiftRequestService(repository.NewShiftRequestRepository(db), repository.NewScheduleRepository(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s, db
}

func createRequest(t *testing.T, s *ShiftRequestService) uint {
	t.Helper()
	v := model.ShiftRequest{ScheduleID: 1, SubstituteID: 2, SubstituteScheduleID: 2, Reason: "家中有事"}
	if e := s.Create(v, 1); e != nil {
		t.Fatalf("create: %v", e)
	}
	var r model.ShiftRequest
	if e := s.db.Last(&r).Error; e != nil {
		t.Fatal(e)
	}
	return r.ID
}

func scheduleOwners(t *testing.T, db *gorm.DB) (uint, uint) {
	t.Helper()
	var a, b model.Schedule
	if e := db.First(&a, 1).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.First(&b, 2).Error; e != nil {
		t.Fatal(e)
	}
	return a.StaffID, b.StaffID
}

func TestShiftRequestConfirmThenReview(t *testing.T) {
	s, db := newShiftRequestService(t)
	id := createRequest(t, s)
	var r model.ShiftRequest
	if e := db.First(&r, id).Error; e != nil {
		t.Fatal(e)
	}
	if r.Status != model.RequestAwaitingSubstitute {
		t.Fatalf("new request status = %s, want %s", r.Status, model.RequestAwaitingSubstitute)
	}
	// 替班人未表态前，主管不能审批。
	if e := s.Review(id, 3, true); e == nil {
		t.Fatal("review before substitute confirm should fail")
	}
	// 非替班人不能确认。
	if e := s.Confirm(id, 1, true); e == nil {
		t.Fatal("confirm by non-substitute should fail")
	}
	// 替班人同意后进入主管审批。
	if e := s.Confirm(id, 2, true); e != nil {
		t.Fatalf("confirm: %v", e)
	}
	if e := db.First(&r, id).Error; e != nil {
		t.Fatal(e)
	}
	if r.Status != model.RequestPending || r.SubstituteConfirmedAt == nil {
		t.Fatalf("after confirm status = %s, confirmed_at = %v", r.Status, r.SubstituteConfirmedAt)
	}
	// 重复确认无效。
	if e := s.Confirm(id, 2, true); e == nil {
		t.Fatal("duplicate confirm should fail")
	}
	// 主管通过后交换双方班次。
	if e := s.Review(id, 3, true); e != nil {
		t.Fatalf("review: %v", e)
	}
	if e := db.First(&r, id).Error; e != nil {
		t.Fatal(e)
	}
	if r.Status != model.RequestApproved || r.ReviewerID == nil || *r.ReviewerID != 3 {
		t.Fatalf("after review status = %s, reviewer = %v", r.Status, r.ReviewerID)
	}
	a, b := scheduleOwners(t, db)
	if a != 2 || b != 1 {
		t.Fatalf("schedules not swapped: %d, %d", a, b)
	}
	// 重复审批不能把班次再换回去。
	if e := s.Review(id, 3, true); e == nil {
		t.Fatal("duplicate review should fail")
	}
	a, b = scheduleOwners(t, db)
	if a != 2 || b != 1 {
		t.Fatalf("schedules swapped back after duplicate review: %d, %d", a, b)
	}
}

func TestShiftRequestDeclinedEndsFlow(t *testing.T) {
	s, db := newShiftRequestService(t)
	id := createRequest(t, s)
	// 替班人拒绝，申请结束。
	if e := s.Confirm(id, 2, false); e != nil {
		t.Fatalf("decline: %v", e)
	}
	var r model.ShiftRequest
	if e := db.First(&r, id).Error; e != nil {
		t.Fatal(e)
	}
	if r.Status != model.RequestDeclined {
		t.Fatalf("status = %s, want %s", r.Status, model.RequestDeclined)
	}
	// 申请已结束，主管不能再审批，班次保持不变。
	if e := s.Review(id, 3, true); e == nil {
		t.Fatal("review after decline should fail")
	}
	a, b := scheduleOwners(t, db)
	if a != 1 || b != 2 {
		t.Fatalf("schedules changed after decline: %d, %d", a, b)
	}
}

func TestShiftRequestLegacyPendingReviewedDirectly(t *testing.T) {
	s, db := newShiftRequestService(t)
	// 模拟流程上线前已提交的申请：直接处于待主管审批状态。
	r := model.ShiftRequest{ApplicantID: 1, SubstituteID: 2, ScheduleID: 1, SubstituteScheduleID: 2, Reason: "历史申请", Status: model.RequestPending}
	if e := db.Create(&r).Error; e != nil {
		t.Fatal(e)
	}
	if e := s.Review(r.ID, 3, true); e != nil {
		t.Fatalf("legacy review: %v", e)
	}
	a, b := scheduleOwners(t, db)
	if a != 2 || b != 1 {
		t.Fatalf("legacy schedules not swapped: %d, %d", a, b)
	}
}
