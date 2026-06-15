package Extractor

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// imageExtractor 调用模型提取图片的Tags作为可索引文本。
func imageExtractor(filePath string) (string, error) {
	// TODO: extract image tags
	return "what", nil
}

// 注册图片文件后缀（常见格式）
func init() {
	imageSuffixes := []string{
		".jpg", ".jpeg", ".png", ".gif", ".bmp",
		".tiff", ".tif", ".webp", ".svg", ".ico",
		//".heic", ".heif", ".jp2", ".psd", ".xcf",		// rare type
	}
	for _, suf := range imageSuffixes {
		register(suf, imageExtractor)
	}
}
