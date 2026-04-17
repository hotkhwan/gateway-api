package kschmod

import (
	"github.com/hotkhwan/gateway-api/models/gmod"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ==== Responses for API ====

type Chat struct {
	ID        primitive.ObjectID  `json:"chatId" bson:"_id,omitempty"`
	ProjectID *primitive.ObjectID `json:"projectId,omitempty" bson:"projectId,omitempty"`
	UserID    string              `json:"userId" bson:"userId"` // 👈 เพิ่ม
	Name      string              `json:"name" bson:"name"`
	CreatedAt time.Time           `json:"createdAt" bson:"createdAt"`
}

type ChatRequest struct {
	ProjectID string `json:"projectId,omitempty"`
	Name      string `json:"name" validate:"required"`
}

type ChatMessage struct {
	ID        primitive.ObjectID  `json:"id" bson:"_id,omitempty"`
	ChatID    primitive.ObjectID  `json:"chatId" bson:"chatId"`
	ProjectID *primitive.ObjectID `json:"projectId,omitempty" bson:"projectId,omitempty"`
	UserID    string              `json:"userId" bson:"userId"` // 👈 เพิ่ม
	Role      string              `json:"role" bson:"role"`     // "user" | "assistant"
	Prompt    string              `json:"prompt,omitempty" bson:"prompt,omitempty"`
	Img       string              `json:"img,omitempty" bson:"img,omitempty"`
	Class     string              `json:"class,omitempty" bson:"class,omitempty"`
	Score     float64             `json:"score,omitempty" bson:"score,omitempty"`
	Meta      interface{}         `json:"meta,omitempty" bson:"meta,omitempty"`
	Caption   string              `json:"caption,omitempty" bson:"caption,omitempty"`
	Summary   string              `json:"summary,omitempty" bson:"summary,omitempty"`
	CreatedAt time.Time           `json:"createdAt" bson:"createdAt"`
}

type ChatListResponse struct {
	gmod.BaseResponse
	Details    []Chat          `json:"details"`
	Pagination gmod.Pagination `json:"pagination"`
}

type ChatsUpdateRequest struct {
	Name string `json:"name" example:"My Updated Project"`
}

// ==== Core Entities ====

type Project struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID    string             `json:"userId" bson:"userId"` // 👈 เพิ่ม
	Name      string             `json:"name" bson:"name"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
}

type ProjectRequest struct {
	Name string `json:"name" validate:"required"`
}

type ProjectChatsResponse struct {
	gmod.BaseResponse
	Details struct {
		Project    *Project        `json:"project"`
		Chats      []Chat          `json:"chats"`
		Pagination gmod.Pagination `json:"pagination"`
	} `json:"details"`
}

type ProjectChatsByIDResponse struct {
	gmod.BaseResponse
	Details struct {
		Project    *Project        `json:"project"`
		Pagination gmod.Pagination `json:"pagination"`
	} `json:"details"`
}

type ProjectListResponse struct {
	gmod.BaseResponse
	Details    []Project       `json:"details"`
	Pagination gmod.Pagination `json:"pagination"`
}

type ProjectUpdateRequest struct {
	Name string `json:"name" example:"My Updated Project"`
}

type LLMConfig struct {
	MyLLMUrl        string
	EnableGPT       bool
	OpenAIKey       string
	OpenAIModel     string
	MaxSummaryToken int
}

type LLMRequest struct {
	Prompt    string `json:"prompt" form:"prompt"`
	ImagePath string `json:"imagePath" form:"imagePath"`
	NameID    string `json:"nameId" form:"nameId"`

	Page      *int   `json:"page,omitempty" form:"page"`         // optional
	PerPage   *int   `json:"perPage,omitempty" form:"perPage"` // optional
	SortOrder string `json:"sortOrder,omitempty" form:"sortOrder"`
	SortField string `json:"sortField,omitempty" form:"sortField"`
}

type LLMDetail struct {
	ID      interface{} `json:"id"`
	Score   float64     `json:"score"`
	PathImg string      `json:"path_img"`
	Class   string      `json:"class"`
	X1      float64     `json:"x1"`
	Y1      float64     `json:"y1"`
	X2      float64     `json:"x2"`
	Y2      float64     `json:"y2"`
	Caption string      `json:"caption"`
}

type LLMResponse struct {
	Status     bool        `json:"status"`
	Details    []LLMDetail `json:"details"`
	Pagination interface{} `json:"pagination"`
}

// ใช้ struct นี้เฉพาะเพื่อให้ Swag เข้าใจว่า T คือ []Device
type PaginationMessageResponse struct {
	Details    []ChatMessage `json:"ChatMessages"`
	Pagination interface{}   `json:"pagination"`
	Status     bool          `json:"status" example:"true"`
}

type Message struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChatID    primitive.ObjectID `bson:"chatId" json:"chatId"`
	UserID    string             `bson:"userId,omitempty" json:"userId,omitempty"`
	Role      string             `bson:"role" json:"role"`
	Prompt    string             `bson:"prompt,omitempty" json:"prompt,omitempty"`
	Caption   string             `bson:"caption,omitempty" json:"caption,omitempty"`
	Img       string             `bson:"img,omitempty" json:"img,omitempty"`
	Class     string             `bson:"class,omitempty" json:"class,omitempty"`
	Score     float64            `bson:"score,omitempty" json:"score,omitempty"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type MessageUpdateRequest struct {
	Prompt  string `json:"prompt,omitempty"`
	Caption string `json:"caption,omitempty"`
	Img     string `json:"img,omitempty"`
	Class   string `json:"class,omitempty"`
}

type ChatMessagePaginationResponse struct {
	Details    []ChatMessage   `json:"details"`
	Pagination gmod.Pagination `json:"pagination"`
	Status     bool            `json:"status"`
}
