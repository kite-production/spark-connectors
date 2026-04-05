// Package msteams provides types and a client for the Microsoft Teams Bot Framework
// and Microsoft Graph API.
package msteams

import "time"

// Activity represents a Bot Framework v3 Activity received from Microsoft Teams.
// Reference: https://learn.microsoft.com/en-us/azure/bot-service/rest-api/bot-framework-rest-connector-api-reference
type Activity struct {
	Type         string        `json:"type"`
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	ServiceURL   string        `json:"serviceUrl"`
	ChannelID    string        `json:"channelId"`
	From         ChannelAccount `json:"from"`
	Conversation ConversationAccount `json:"conversation"`
	Recipient    ChannelAccount `json:"recipient"`
	Text         string        `json:"text"`
	TextFormat   string        `json:"textFormat"`
	ReplyToID    string        `json:"replyToId,omitempty"`
	ChannelData  *ChannelData  `json:"channelData,omitempty"`
}

// ChannelAccount identifies a user or bot in Teams.
type ChannelAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	AADObjectID string `json:"aadObjectId,omitempty"`
}

// ConversationAccount identifies a conversation in Teams.
type ConversationAccount struct {
	ID               string `json:"id"`
	Name             string `json:"name,omitempty"`
	ConversationType string `json:"conversationType,omitempty"`
	IsGroup          bool   `json:"isGroup,omitempty"`
	TenantID         string `json:"tenantId,omitempty"`
}

// ChannelData holds Teams-specific data included in the activity.
type ChannelData struct {
	Team    *TeamInfo    `json:"team,omitempty"`
	Channel *ChannelInfo `json:"channel,omitempty"`
	Tenant  *TenantInfo  `json:"tenant,omitempty"`
}

// TeamInfo identifies the team a message belongs to.
type TeamInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// ChannelInfo identifies the channel within a team.
type ChannelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// TenantInfo identifies the Azure AD tenant.
type TenantInfo struct {
	ID string `json:"id"`
}

// TokenResponse is the OAuth2 token response from Azure AD.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// GraphMessageResponse is the response from the Graph API when sending a message.
type GraphMessageResponse struct {
	ID string `json:"id"`
}
