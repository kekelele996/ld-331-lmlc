package repository

import (
	"errors"
	"github.com/gbsched/hospital-scheduler/internal/model"
	"gorm.io/gorm"
)

type ShiftRequestRepository struct{ db *gorm.DB }

func NewShiftRequestRepository(db *gorm.DB) *ShiftRequestRepository {
	return &ShiftRequestRepository{db}
}
func (r *ShiftRequestRepository) Create(v *model.ShiftRequest) error { return r.db.Create(v).Error }
func (r *ShiftRequestRepository) Save(v *model.ShiftRequest) error   { return r.db.Save(v).Error }

// Transition 仅当申请仍处于 from 状态时才执行更新，返回受影响行数；
// 调用方依据行数判断是否为重复/并发操作，避免状态被覆盖或班次被反复交换。
func (r *ShiftRequestRepository) Transition(id uint, from model.RequestStatus, updates map[string]any) (int64, error) {
	res := r.db.Model(&model.ShiftRequest{}).Where("id = ? AND status = ?", id, from).Updates(updates)
	return res.RowsAffected, res.Error
}
func (r *ShiftRequestRepository) Find(id uint) (model.ShiftRequest, error) {
	var v model.ShiftRequest
	e := r.db.Preload("Applicant").Preload("Substitute").First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return v, ErrNotFound
	}
	return v, e
}
func (r *ShiftRequestRepository) List() ([]model.ShiftRequest, error) {
	var v []model.ShiftRequest
	return v, r.db.Preload("Applicant").Preload("Substitute").Order("id desc").Find(&v).Error
}
