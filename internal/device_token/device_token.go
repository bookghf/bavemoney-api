package device_token

type DeviceToken struct {
	ID        string `json:"id"`
	FCMToken  string `json:"fcm_token"`
	Platform  string `json:"platform"`
	CreatedAt string `json:"created_at"`
}

type CreateRequest struct {
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"`
}
