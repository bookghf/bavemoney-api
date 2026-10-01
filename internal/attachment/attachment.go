package attachment

type Attachment struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	FileURL       string `json:"file_url"`
	FileSizeBytes int    `json:"file_size_bytes"`
	UploadedAt    string `json:"uploaded_at"`
}

type PresignedURLRequest struct {
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int    `json:"file_size_bytes"`
}

type PresignedURLResponse struct {
	AttachmentID     string            `json:"attachment_id"`
	UploadURL        string            `json:"upload_url"`
	UploadMethod     string            `json:"upload_method"`
	UploadHeaders    map[string]string `json:"upload_headers"`
	ExpiresInSeconds int               `json:"expires_in_seconds"`
}
