package services

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-im/internal/config"
	"go-im/internal/models"
	"go-im/internal/store/sqlstore"

	"github.com/google/uuid"
)

// 文件服务
type FileService struct {
	DB        *sqlstore.Stores
	UploadDir string // 本地上传目录（当未启用云存储时）
	BaseURL   string // 本地文件访问基础 URL
	MaxSize   int64  // 最大文件大小（字节）
	Cfg       *config.Config
}

func NewFileService(db *sqlstore.Stores, uploadDir, baseURL string, maxSize int64) *FileService {
	return &FileService{
		DB:        db,
		UploadDir: uploadDir,
		BaseURL:   baseURL,
		MaxSize:   maxSize,
	}
}

func (s *FileService) WithConfig(cfg *config.Config) *FileService { s.Cfg = cfg; return s }

// 生成阿里云 OSS 直传签名
// 前端使用：POST 到 https://{bucket}.{endpoint}/，表单字段包含 key, policy, OSSAccessKeyId, signature, success_action_status, ...
func (s *FileService) GenerateOSSPolicy(ctx context.Context, userID string, dir string) (map[string]interface{}, error) {
	if s.Cfg == nil || !s.Cfg.OSSEnabled {
		return nil, fmt.Errorf("OSS not enabled")
	}

	bucket := s.Cfg.OSSBucket
	endpoint := s.Cfg.OSSEndpoint
	accessKey := s.Cfg.OSSAccessKeyID
	secretKey := s.Cfg.OSSAccessKeySecret
	if bucket == "" || endpoint == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("incomplete OSS config")
	}

	// key 前缀：prefix/yyyy/mm/dd/{uuid}
	if dir == "" {
		dir = s.Cfg.OSSPrefix
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	now := time.Now()
	datePrefix := fmt.Sprintf("%04d/%02d/%02d/", now.Year(), now.Month(), now.Day())
	keyPrefix := dir + datePrefix

	maxSize := s.Cfg.OSSMaxSizeMB * 1024 * 1024
	expire := time.Now().Add(time.Duration(s.Cfg.OSSExpireSeconds) * time.Second).UTC().Format(time.RFC3339)

	// policy 条件
	policy := map[string]interface{}{
		"expiration": expire,
		"conditions": []interface{}{
			map[string]string{"bucket": bucket},
			[]interface{}{"starts-with", "$key", keyPrefix},
			[]interface{}{"content-length-range", 0, maxSize},
		},
	}
	policyJSON, _ := json.Marshal(policy)
	policyBase64 := base64.StdEncoding.EncodeToString(policyJSON)

	// 签名
	h := hmac.New(sha1.New, []byte(secretKey))
	h.Write([]byte(policyBase64))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Host（供前端直传使用）	host := fmt.Sprintf("https://%s.%s", bucket, endpoint)
	host := fmt.Sprintf("https://%s.%s", bucket, endpoint)
	if s.Cfg.OSSPublicHost != "" {
		host = s.Cfg.OSSPublicHost
	}

	resp := map[string]interface{}{
		"accessKeyId": accessKey,
		"policy":      policyBase64,
		"signature":   signature,
		"dir":         keyPrefix,
		"host":        host,
		"expireAt":    expire,
		"maxSize":     maxSize,
	}
	return resp, nil
}

// OSS 回调校验与入库（可选：如果配置了 OSS 回调）
func (s *FileService) ConfirmOSSCallback(ctx context.Context, userID string, key string, size int64, mimeType string) (*models.FileUpload, error) {
	if s.Cfg == nil || !s.Cfg.OSSEnabled {
		return nil, fmt.Errorf("OSS not enabled")
	}
	url := s.Cfg.OSSPublicHost
	if url == "" {
		url = fmt.Sprintf("https://%s.%s", s.Cfg.OSSBucket, s.Cfg.OSSEndpoint)
	}
	url = strings.TrimRight(url, "/") + "/" + strings.TrimLeft(key, "/")

	upload := &models.FileUpload{
		ID:        uuid.NewString(),
		UserID:    userID,
		FileName:  filepath.Base(key),
		FileSize:  size,
		MimeType:  mimeType,
		StorePath: key,
		URL:       url,
		Status:    "success",
		CreatedAt: time.Now(),
	}
	if err := s.createFileRecord(ctx, upload); err != nil {
		return nil, err
	}
	return upload, nil
}

// 上传文件
func (s *FileService) UploadFile(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader) (*models.FileUpload, error) {
	// 检查文件大小
	if header.Size > s.MaxSize {
		return nil, fmt.Errorf("file size exceeds limit: %d bytes", s.MaxSize)
	}

	// 生成文件 ID
	fileID := uuid.NewString()

	// 获取文件扩展名
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".bin"
	}

	// 生成存储路径：uploads/2024/01/02/uuid.ext
	now := time.Now()
	relativeDir := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	storageDir := filepath.Join(s.UploadDir, relativeDir)

	// 确保目录存在
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 生成唯一文件名
	fileName := fileID + ext
	filePath := filepath.Join(storageDir, fileName)
	relativePath := filepath.Join(relativeDir, fileName)

	// 创建文件记录
	upload := &models.FileUpload{
		ID:        fileID,
		UserID:    userID,
		FileName:  header.Filename,
		FileSize:  header.Size,
		MimeType:  header.Header.Get("Content-Type"),
		StorePath: relativePath,
		URL:       s.BaseURL + "/" + strings.ReplaceAll(relativePath, "\\", "/"),
		Status:    "uploading",
		CreatedAt: now,
	}

	// 保存到数据库
	if err := s.createFileRecord(ctx, upload); err != nil {
		return nil, fmt.Errorf("failed to create file record: %w", err)
	}

	// 保存文件
	dst, err := os.Create(filePath)
	if err != nil {
		s.updateFileStatus(ctx, fileID, "failed")
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		s.updateFileStatus(ctx, fileID, "failed")
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// 更新状态为成功
	upload.Status = "success"
	s.updateFileStatus(ctx, fileID, "success")

	return upload, nil
}

// 获取文件信息
func (s *FileService) GetFile(ctx context.Context, fileID string) (*models.FileUpload, error) {
	query := `SELECT id, user_id, file_name, file_size, mime_type, store_path, url, status, created_at, expires_at 
			  FROM file_uploads WHERE id = ?`

	var upload models.FileUpload
	err := s.DB.Primary.QueryRowContext(ctx, query, fileID).Scan(
		&upload.ID, &upload.UserID, &upload.FileName, &upload.FileSize,
		&upload.MimeType, &upload.StorePath, &upload.URL, &upload.Status,
		&upload.CreatedAt, &upload.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}

	return &upload, nil
}

// 删除文件
func (s *FileService) DeleteFile(ctx context.Context, fileID, userID string) error {
	// 获取文件信息
	upload, err := s.GetFile(ctx, fileID)
	if err != nil {
		return err
	}

	// 检查权限
	if upload.UserID != userID {
		return fmt.Errorf("permission denied")
	}

	// 删除物理文件
	filePath := filepath.Join(s.UploadDir, upload.StorePath)
	os.Remove(filePath)

	// 删除数据库记录
	query := `DELETE FROM file_uploads WHERE id = ?`
	_, err = s.DB.Primary.ExecContext(ctx, query, fileID)
	return err
}

// 获取用户文件列表
func (s *FileService) ListUserFiles(ctx context.Context, userID string, limit, offset int) ([]*models.FileUpload, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `SELECT id, user_id, file_name, file_size, mime_type, store_path, url, status, created_at, expires_at 
			  FROM file_uploads WHERE user_id = ? AND status = 'success' 
			  ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.DB.Primary.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []*models.FileUpload
	for rows.Next() {
		var upload models.FileUpload
		err := rows.Scan(&upload.ID, &upload.UserID, &upload.FileName, &upload.FileSize,
			&upload.MimeType, &upload.StorePath, &upload.URL, &upload.Status,
			&upload.CreatedAt, &upload.ExpiresAt)
		if err != nil {
			continue
		}
		uploads = append(uploads, &upload)
	}

	return uploads, nil
}

// 生成文件哈希
func (s *FileService) generateFileHash(file multipart.File) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// 创建文件记录
func (s *FileService) createFileRecord(ctx context.Context, upload *models.FileUpload) error {
	query := `INSERT INTO file_uploads (id, user_id, file_name, file_size, mime_type, store_path, url, status, created_at, expires_at) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.DB.Primary.ExecContext(ctx, query,
		upload.ID, upload.UserID, upload.FileName, upload.FileSize,
		upload.MimeType, upload.StorePath, upload.URL, upload.Status,
		upload.CreatedAt, upload.ExpiresAt)

	return err
}

// 更新文件状态
func (s *FileService) updateFileStatus(ctx context.Context, fileID, status string) error {
	query := `UPDATE file_uploads SET status = ? WHERE id = ?`
	_, err := s.DB.Primary.ExecContext(ctx, query, status, fileID)
	return err
}

// 清理过期文件（定时任务）
func (s *FileService) CleanupExpiredFiles(ctx context.Context) error {
	// 查询过期文件
	query := `SELECT id, store_path FROM file_uploads 
			  WHERE expires_at IS NOT NULL AND expires_at < NOW()`

	rows, err := s.DB.Primary.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var files []struct {
		ID        string
		StorePath string
	}

	for rows.Next() {
		var file struct {
			ID        string
			StorePath string
		}
		if err := rows.Scan(&file.ID, &file.StorePath); err != nil {
			continue
		}
		files = append(files, file)
	}

	// 删除过期文件
	for _, file := range files {
		// 删除物理文件
		filePath := filepath.Join(s.UploadDir, file.StorePath)
		os.Remove(filePath)

		// 删除数据库记录
		deleteQuery := `DELETE FROM file_uploads WHERE id = ?`
		s.DB.Primary.ExecContext(ctx, deleteQuery, file.ID)
	}

	return nil
}
