package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

// GrokVideoRequestError identifies invalid client video parameters before account
// selection or billing. It must not be treated as an upstream account failure.
type GrokVideoRequestError struct{ Message string }

func (e *GrokVideoRequestError) Error() string { return e.Message }

func grokVideoRequestError(format string, args ...any) error {
	return &GrokVideoRequestError{Message: fmt.Sprintf(format, args...)}
}

// PrepareGrokVideoGenerationRequest adapts OpenAI video clients to xAI's JSON
// generation protocol. Native JSON options are retained, and explicit invalid
// duration values are rejected rather than silently changing the requested clip.
func PrepareGrokVideoGenerationRequest(body []byte, contentType string) ([]byte, string, error) {
	mediaType := "application/json"
	var params map[string]string
	if strings.TrimSpace(contentType) != "" {
		var err error
		mediaType, params, err = mime.ParseMediaType(contentType)
		if err != nil {
			return nil, "", grokVideoRequestError("Invalid video request Content-Type")
		}
	}
	var payload map[string]json.RawMessage
	multipartBody := strings.EqualFold(mediaType, "multipart/form-data")
	if multipartBody {
		var err error
		payload, err = parseGrokVideoMultipart(body, params["boundary"])
		if err != nil {
			return nil, "", err
		}
	} else {
		if !strings.EqualFold(mediaType, "application/json") {
			return nil, "", grokVideoRequestError("Video requests require application/json or multipart/form-data")
		}
		var err error
		payload, err = parseGrokVideoJSONObject(body)
		if err != nil {
			return nil, "", err
		}
	}
	changed := multipartBody
	// Canvas clients label the resolution separately from their pixel geometry.
	// Keep an explicit native resolution authoritative and remove the alias.
	if raw, exists := payload["resolution_name"]; exists {
		if _, native := payload["resolution"]; !native {
			payload["resolution"] = raw
		}
		delete(payload, "resolution_name")
		changed = true
	}
	for _, field := range []string{"model", "prompt", "aspect_ratio"} {
		if raw, ok := payload[field]; ok {
			if _, err := grokVideoString(raw, field); err != nil {
				return nil, "", err
			}
		}
	}
	if raw, exists := payload["aspect_ratio"]; exists {
		value, _ := grokVideoString(raw, "aspect_ratio")
		if !grokVideoAspectRatioSupported(value) {
			return nil, "", grokVideoRequestError("aspect_ratio must be 1:1, 16:9, 9:16, 4:3, 3:4, 3:2, or 2:3")
		}
	}
	var duration, seconds int
	for _, field := range []string{"duration", "seconds"} {
		if raw, ok := payload[field]; ok {
			value, err := grokVideoDuration(raw, field, multipartBody || field == "seconds")
			if err != nil {
				return nil, "", err
			}
			if field == "duration" {
				duration = value
			} else {
				seconds = value
			}
		}
	}
	if duration != 0 && seconds != 0 && duration != seconds {
		return nil, "", grokVideoRequestError("duration and seconds must specify the same value")
	}
	if seconds != 0 {
		duration = seconds
		delete(payload, "seconds")
		changed = true
	}
	if duration != 0 && (changed || multipartBody) {
		payload["duration"] = json.RawMessage(strconv.Itoa(duration))
	}
	if raw, ok := payload["resolution"]; ok {
		value, err := grokVideoString(raw, "resolution")
		if err != nil {
			return nil, "", err
		}
		resolution, valid := LookupVideoBillingResolution(value)
		if !valid {
			return nil, "", grokVideoRequestError("resolution must be 480p, 720p, or 1080p")
		}
		if resolution != value {
			payload["resolution"] = grokVideoJSONValue(resolution)
			changed = true
		}
	}
	if raw, ok := payload["size"]; ok {
		size, err := grokVideoString(raw, "size")
		if err != nil {
			return nil, "", err
		}
		if size != "" && !strings.EqualFold(size, "auto") {
			width, height, valid := parseImageBillingDimensions(size)
			if !valid || width <= 0 || height <= 0 {
				return nil, "", grokVideoRequestError("size must be WIDTHxHEIGHT or auto")
			}
			if _, exists := payload["aspect_ratio"]; !exists {
				payload["aspect_ratio"] = grokVideoJSONValue(grokVideoAspectRatioFromDimensions(width, height))
			}
			if _, exists := payload["resolution"]; !exists {
				shortSide := min(width, height)
				resolution := VideoBillingResolution480P
				if shortSide >= 1080 {
					resolution = VideoBillingResolution1080P
				} else if shortSide >= 720 {
					resolution = VideoBillingResolution720P
				}
				payload["resolution"] = grokVideoJSONValue(resolution)
			}
		}
		delete(payload, "size")
		changed = true
	}
	if raw, exists := payload["input_reference[]"]; exists {
		for _, field := range []string{"image", "input_reference", "image_url", "images", "reference_images"} {
			if _, conflict := payload[field]; conflict {
				return nil, "", grokVideoRequestError("Specify input_reference[] or %s, not both", field)
			}
		}
		var refs []json.RawMessage
		if json.Unmarshal(raw, &refs) != nil || len(refs) == 0 || len(refs) > 7 {
			return nil, "", grokVideoRequestError("input_reference[] must contain between 1 and 7 images")
		}
		images := make([]map[string]string, 0, len(refs))
		for _, ref := range refs {
			imageURL, err := grokVideoReferenceURL(ref, "input_reference[]")
			if err != nil {
				return nil, "", err
			}
			images = append(images, map[string]string{"url": imageURL})
		}
		if len(images) == 1 {
			payload["image"] = grokVideoJSONValue(images[0])
		} else {
			payload["reference_images"] = grokVideoJSONValue(images)
		}
		delete(payload, "input_reference[]")
		changed = true
	}
	for _, alias := range []string{"input_reference", "image_url"} {
		raw, ok := payload[alias]
		if !ok {
			continue
		}
		if _, exists := payload["image"]; exists {
			return nil, "", grokVideoRequestError("Specify only one of image, input_reference, and image_url")
		}
		imageURL, err := grokVideoReferenceURL(raw, alias)
		if err != nil {
			return nil, "", err
		}
		payload["image"] = grokVideoJSONValue(map[string]string{"url": imageURL})
		delete(payload, alias)
		changed = true
	}
	if !changed {
		return body, "application/json", nil
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, "", grokVideoRequestError("Invalid video request JSON")
	}
	return out, "application/json", nil
}

func parseGrokVideoJSONObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, grokVideoRequestError("Video request body must be a JSON object")
	}
	payload := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, grokVideoRequestError("Invalid video request JSON")
		}
		name, ok := key.(string)
		if !ok {
			return nil, grokVideoRequestError("Invalid video request JSON")
		}
		if _, exists := payload[name]; exists {
			return nil, grokVideoRequestError("Duplicate video JSON field: %s", name)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, grokVideoRequestError("Invalid video request JSON")
		}
		payload[name] = raw
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, grokVideoRequestError("Invalid video request JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, grokVideoRequestError("Invalid video request JSON")
	}
	return payload, nil
}

func grokVideoAspectRatioSupported(value string) bool {
	switch value {
	case "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3":
		return true
	default:
		return false
	}
}

func grokVideoAspectRatioFromDimensions(width, height int) string {
	ratio := float64(width) / float64(height)
	best, distance := "1:1", math.MaxFloat64
	for _, candidate := range grokImagineAspectRatioValues {
		if !grokVideoAspectRatioSupported(candidate.label) {
			continue
		}
		if delta := math.Abs(ratio - candidate.ratio); delta < distance {
			best, distance = candidate.label, delta
		}
	}
	return best
}

func grokVideoJSONValue(value any) json.RawMessage {
	out, _ := json.Marshal(value)
	return out
}

func grokVideoString(raw json.RawMessage, field string) (string, error) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return "", grokVideoRequestError("%s must be a string", field)
	}
	return value, nil
}

func grokVideoDuration(raw json.RawMessage, field string, allowString bool) (int, error) {
	value := string(bytes.TrimSpace(raw))
	if allowString && strings.HasPrefix(value, `"`) {
		parsed, err := grokVideoString(raw, field)
		if err != nil {
			return 0, err
		}
		value = strings.TrimSpace(parsed)
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < VideoBillingMinDurationSeconds || seconds > VideoBillingMaxDurationSeconds {
		return 0, grokVideoRequestError("%s must be an integer between %d and %d seconds", field, VideoBillingMinDurationSeconds, VideoBillingMaxDurationSeconds)
	}
	return seconds, nil
}

func grokVideoReferenceURL(raw json.RawMessage, field string) (string, error) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", grokVideoRequestError("%s must be an image URL or image URL object", field)
	}
	var imageURL string
	switch typed := value.(type) {
	case string:
		imageURL = typed
	case map[string]any:
		imageURL, _ = typed["url"].(string)
		if imageURL == "" {
			imageURL, _ = typed["image_url"].(string)
		}
	}
	if strings.TrimSpace(imageURL) == "" {
		return "", grokVideoRequestError("%s must contain a nonempty image URL", field)
	}
	return strings.TrimSpace(imageURL), nil
}

func parseGrokVideoMultipart(body []byte, boundary string) (map[string]json.RawMessage, error) {
	if strings.TrimSpace(boundary) == "" {
		return nil, grokVideoRequestError("Video multipart request is missing its boundary")
	}
	payload := make(map[string]json.RawMessage)
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return payload, nil
		}
		if err != nil {
			return nil, grokVideoRequestError("Malformed video multipart request")
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			return nil, grokVideoRequestError("Video multipart fields must have a name")
		}
		if _, exists := payload[name]; exists && name != "input_reference[]" {
			_ = part.Close()
			return nil, grokVideoRequestError("Duplicate video multipart field: %s", name)
		}
		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize+1))
		_ = part.Close()
		if err != nil {
			return nil, grokVideoRequestError("Malformed video multipart field: %s", name)
		}
		if len(data) > openAIImageMaxUploadPartSize {
			return nil, grokVideoRequestError("Video multipart field %s exceeds the 20 MB limit", name)
		}
		if fileName := part.FileName(); fileName != "" {
			if name != "input_reference" && name != "input_reference[]" && name != "image" {
				return nil, grokVideoRequestError("Unsupported video upload field: %s", name)
			}
			detectedType := http.DetectContentType(data)
			if !strings.HasPrefix(detectedType, "image/") {
				return nil, grokVideoRequestError("%s must be an image upload", name)
			}
			imageURL, err := openAIImageUploadToDataURL(OpenAIImagesUpload{FieldName: name, FileName: fileName, ContentType: detectedType, Data: data})
			if err != nil {
				return nil, grokVideoRequestError("Invalid %s image upload", name)
			}
			if name == "input_reference[]" {
				if err := appendGrokVideoReference(payload, grokVideoJSONValue(map[string]string{"url": imageURL})); err != nil {
					return nil, err
				}
			} else {
				payload[name] = grokVideoJSONValue(map[string]string{"url": imageURL})
			}
			continue
		}
		value := string(data)
		switch name {
		case "input_reference[]":
			ref := grokVideoJSONValue(value)
			if strings.HasPrefix(strings.TrimSpace(value), "{") && json.Valid(data) {
				ref = json.RawMessage(data)
			}
			if err := appendGrokVideoReference(payload, ref); err != nil {
				return nil, err
			}
		case "image", "input_reference":
			if strings.HasPrefix(strings.TrimSpace(value), "{") && json.Valid(data) {
				payload[name] = json.RawMessage(data)
			} else if name == "image" {
				payload[name] = grokVideoJSONValue(map[string]string{"url": strings.TrimSpace(value)})
			} else {
				payload[name] = grokVideoJSONValue(value)
			}
		case "images", "reference_images":
			var refs []json.RawMessage
			if err := json.Unmarshal(data, &refs); err != nil || refs == nil {
				return nil, grokVideoRequestError("%s must be a JSON array", name)
			}
			payload[name] = json.RawMessage(data)
		default:
			payload[name] = grokVideoJSONValue(value)
		}
	}
}

func appendGrokVideoReference(payload map[string]json.RawMessage, ref json.RawMessage) error {
	var refs []json.RawMessage
	if raw, exists := payload["input_reference[]"]; exists {
		if err := json.Unmarshal(raw, &refs); err != nil {
			return grokVideoRequestError("Invalid input_reference[] images")
		}
	}
	if len(refs) >= 7 {
		return grokVideoRequestError("input_reference[] supports at most 7 images")
	}
	payload["input_reference[]"] = grokVideoJSONValue(append(refs, ref))
	return nil
}
