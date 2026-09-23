package service

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gbsched/hospital-scheduler/internal/model"
	"github.com/gbsched/hospital-scheduler/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRequestTest(t *testing.T) (*gorm.DB, *ShiftRequestService, model.Staff, model.Staff) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&model.Staff{}, &model.Shift{}, &model.Schedule{}, &model.ShiftRequest{}); e != nil {
		t.Fatal(e)
	}
	applicant := model.Staff{Name: "A", Username: "a-user", Active: true, Role: model.RoleStaff}
	substitute := model.Staff{Name: "B", Username: "b-user", Active: true, Role: model.RoleStaff}
	if e = db.Create(&applicant).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&substitute).Error; e != nil {
		t.Fatal(e)
	}
	shift := model.Shift{Name: "day", Kind: model.ShiftDay}
	if e = db.Create(&shift).Error; e != nil {
		t.Fatal(e)
	}
	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)
	schedA := model.Schedule{DepartmentID: 1, StaffID: applicant.ID, ShiftID: shift.ID, WorkDate: day}
	schedB := model.Schedule{DepartmentID: 1, StaffID: substitute.ID, ShiftID: shift.ID, WorkDate: day.AddDate(0, 0, 1)}
	if e = db.Create(&schedA).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&schedB).Error; e != nil {
		t.Fatal(e)
	}
	svc := NewShiftRequestService(
		repository.NewShiftRequestRepository(db),
		repository.NewScheduleRepository(db),
		db,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return db, svc, applicant, substitute
}

func createRequest(t *testing.T, db *gorm.DB, svc *ShiftRequestService, applicant, substitute model.Staff) model.ShiftRequest {
	t.Helper()
	var a, b model.Schedule
	db.First(&a, "staff_id = ?", applicant.ID)
	db.First(&b, "staff_id = ?", substitute.ID)
	req := model.ShiftRequest{ScheduleID: a.ID, SubstituteScheduleID: b.ID, SubstituteID: substitute.ID, Reason: "身体不适需要换班"}
	if e := svc.Create(req, applicant.ID); e != nil {
		t.Fatalf("create: %v", e)
	}
	created, e := svc.repo.Find(1)
	if e != nil {
		t.Fatal(e)
	}
	return created
}

func TestShiftRequest_TwoStageApproval(t *testing.T) {
	db, svc, applicant, substitute := setupRequestTest(t)
	r := createRequest(t, db, svc, applicant, substitute)

	if r.Status != model.RequestSubstitutePending {
		t.Fatalf("new request status = %s, want substitute_pending", r.Status)
	}
	// 主管在替班人表态前不能审批
	if e := svc.Review(r.ID, 99, true); !errors.Is(e, ErrWrongStage) {
		t.Fatalf("early review err = %v, want ErrWrongStage", e)
	}
	// 其他人不能代替替班人表态
	if e := svc.Respond(r.ID, applicant.ID, true); !errors.Is(e, ErrNotSubstitute) {
		t.Fatalf("respond by other err = %v, want ErrNotSubstitute", e)
	}
	if e := svc.Respond(r.ID, substitute.ID, true); e != nil {
		t.Fatalf("substitute approve: %v", e)
	}
	r, _ = svc.repo.Find(r.ID)
	if r.Status != model.RequestSupervisorPending || r.SubstituteDecidedAt == nil {
		t.Fatalf("after respond status = %s, want supervisor_pending with decided_at", r.Status)
	}
	// 替班人重复表态无效（申请已流转到主管环节，不再处于替班人阶段）
	if e := svc.Respond(r.ID, substitute.ID, false); !errors.Is(e, ErrWrongStage) {
		t.Fatalf("duplicate respond err = %v, want ErrWrongStage", e)
	}
	if e := svc.Review(r.ID, 99, true); e != nil {
		t.Fatalf("supervisor approve: %v", e)
	}
	r, _ = svc.repo.Find(r.ID)
	if r.Status != model.RequestApproved {
		t.Fatalf("final status = %s, want approved", r.Status)
	}
	var a, b model.Schedule
	db.First(&a, r.ScheduleID)
	db.First(&b, r.SubstituteScheduleID)
	if a.StaffID != substitute.ID || b.StaffID != applicant.ID {
		t.Fatalf("schedules not swapped: a=%d b=%d", a.StaffID, b.StaffID)
	}
	// 重复审批不能把班次再换回来
	if e := svc.Review(r.ID, 99, true); !errors.Is(e, ErrRequestClosed) {
		t.Fatalf("duplicate review err = %v, want ErrRequestClosed", e)
	}
	db.First(&a, r.ScheduleID)
	db.First(&b, r.SubstituteScheduleID)
	if a.StaffID != substitute.ID || b.StaffID != applicant.ID {
		t.Fatalf("schedules swapped again by duplicate review")
	}
}

func TestShiftRequest_SubstituteRejectionEndsFlow(t *testing.T) {
	db, svc, applicant, substitute := setupRequestTest(t)
	r := createRequest(t, db, svc, applicant, substitute)

	if e := svc.Respond(r.ID, substitute.ID, false); e != nil {
		t.Fatalf("substitute reject: %v", e)
	}
	r, _ = svc.repo.Find(r.ID)
	if r.Status != model.RequestSubstituteRejected {
		t.Fatalf("status = %s, want substitute_rejected", r.Status)
	}
	// 申请已结束，主管无法再审批，班次保持不变
	if e := svc.Review(r.ID, 99, true); !errors.Is(e, ErrRequestClosed) {
		t.Fatalf("review after reject err = %v, want ErrRequestClosed", e)
	}
	var a, b model.Schedule
	db.First(&a, r.ScheduleID)
	db.First(&b, r.SubstituteScheduleID)
	if a.StaffID != applicant.ID || b.StaffID != substitute.ID {
		t.Fatalf("schedules must remain unchanged after rejection")
	}
}

func TestShiftRequest_LegacyPendingReviewedDirectly(t *testing.T) {
	db, svc, applicant, substitute := setupRequestTest(t)
	r := createRequest(t, db, svc, applicant, substitute)
	// 模拟改造前已提交、直接处于 pending 的历史申请
	if e := db.Model(&model.ShiftRequest{}).Where("id = ?", r.ID).
		Update("status", model.RequestPending).Error; e != nil {
		t.Fatal(e)
	}
	// 历史申请不再经过替班人确认，主管直接审批通过并交换班次
	if e := svc.Respond(r.ID, substitute.ID, true); !errors.Is(e, ErrWrongStage) {
		t.Fatalf("legacy respond err = %v, want ErrWrongStage", e)
	}
	if e := svc.Review(r.ID, 99, true); e != nil {
		t.Fatalf("legacy review: %v", e)
	}
	r, _ = svc.repo.Find(r.ID)
	if r.Status != model.RequestApproved {
		t.Fatalf("legacy status = %s, want approved", r.Status)
	}
}
