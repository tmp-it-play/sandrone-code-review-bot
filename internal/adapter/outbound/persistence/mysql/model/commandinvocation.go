package model

import "time"

type CommandInvocation struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	InvocationKey *string `gorm:"size:64;uniqueIndex"`
	Owner         string  `gorm:"size:100;index:idx_command_target,priority:1"`
	Repository    string  `gorm:"size:150;index:idx_command_target,priority:2"`
	Number        int     `gorm:"index:idx_command_target,priority:3"`
	Invoker       string  `gorm:"size:150"`
	Kind          string  `gorm:"size:40"`
	Allowed       bool
	Detail        string    `gorm:"type:text"`
	OccurredAt    time.Time `gorm:"index"`
}

func (CommandInvocation) TableName() string {
	return "command_invocations"
}
