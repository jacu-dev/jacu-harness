package workspace

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

const reviewReceiptMaxAge = 24 * time.Hour

// ReviewReceipt is deliberately limited to facts this runtime can attest:
// verdict, digest, reasons, and timestamp. It has no reviewer-session claim.
type ReviewReceipt struct {
	RunID      string    `json:"run_id"`
	DiffDigest string    `json:"diff_digest"`
	Verdict    string    `json:"verdict"`
	Reasons    []string  `json:"reasons"`
	CreatedAt  time.Time `json:"created_at"`
	Signature  string    `json:"signature"`
}

type reviewReceiptPayload struct {
	RunID      string    `json:"run_id"`
	DiffDigest string    `json:"diff_digest"`
	Verdict    string    `json:"verdict"`
	Reasons    []string  `json:"reasons"`
	CreatedAt  time.Time `json:"created_at"`
}

func SignReviewReceipt(receipt ReviewReceipt, key []byte) (ReviewReceipt, error) {
	if len(key) == 0 {
		return ReviewReceipt{}, errors.New("receipt signing key is empty")
	}
	if err := validateReceiptFields(receipt); err != nil {
		return ReviewReceipt{}, err
	}
	receipt.Reasons = append([]string{}, receipt.Reasons...)
	payload, err := json.Marshal(reviewReceiptPayloadFrom(receipt))
	if err != nil {
		return ReviewReceipt{}, fmt.Errorf("marshal receipt payload: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	receipt.Signature = hex.EncodeToString(mac.Sum(nil))
	return receipt, nil
}

func WriteReviewReceipt(root string, receipt ReviewReceipt) (string, error) {
	if !runstate.ValidRunID(receipt.RunID) {
		return "", fmt.Errorf("invalid receipt run_id %q", receipt.RunID)
	}
	if receipt.Signature == "" {
		return "", errors.New("receipt signature is empty")
	}
	dir := filepath.Join(root, ".git", "jacu", "receipts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create receipt directory: %w", err)
	}
	path := filepath.Join(dir, receipt.RunID+".json")
	content, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal receipt: %w", err)
	}
	content = append(content, '\n')
	// #nosec G304 -- path is constrained to .git/jacu/receipts and a validated run id.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create receipt: %w", err)
	}
	if _, writeErr := file.Write(content); writeErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write receipt: %w", writeErr)
	}
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("sync receipt: %w", syncErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close receipt: %w", closeErr)
	}
	return path, nil
}

// LoadOrCreateReceiptKey keeps the local HMAC key inside the repository's
// private Jacu metadata. The key is never returned in an API result or log.
func LoadOrCreateReceiptKey(root string) ([]byte, error) {
	dir := filepath.Join(root, ".git", "jacu")
	path := filepath.Join(dir, "receipt.key")
	// #nosec G304 -- path is constrained to the repository's private receipt key.
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) < 32 {
			return nil, errors.New("receipt key is too short")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read receipt key: %w", err)
	}
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return nil, fmt.Errorf("create receipt key directory: %w", mkdirErr)
	}
	key = make([]byte, 32)
	if _, randomErr := rand.Read(key); randomErr != nil {
		return nil, fmt.Errorf("generate receipt key: %w", randomErr)
	}
	// #nosec G304 -- path is constrained to the repository's private receipt key.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return LoadOrCreateReceiptKey(root)
		}
		return nil, fmt.Errorf("create receipt key: %w", err)
	}
	if _, writeErr := file.Write(key); writeErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write receipt key: %w", writeErr)
	}
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync receipt key: %w", syncErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close receipt key: %w", closeErr)
	}
	return key, nil
}

func ConsumeReviewReceipt(root, runID, diffDigest string, key []byte, now time.Time) (ReviewReceipt, error) {
	if !runstate.ValidRunID(runID) {
		return ReviewReceipt{}, fmt.Errorf("invalid receipt run_id %q", runID)
	}
	path := filepath.Join(root, ".git", "jacu", "receipts", runID+".json")
	// #nosec G304 -- path is constrained to .git/jacu/receipts and a validated run id.
	content, err := os.ReadFile(path)
	if err != nil {
		return ReviewReceipt{}, fmt.Errorf("read receipt: %w", err)
	}
	receipt, err := decodeReviewReceipt(content)
	if err != nil {
		return ReviewReceipt{}, err
	}
	if validateErr := ValidateReviewReceipt(receipt, key, runID, diffDigest, now); validateErr != nil {
		return ReviewReceipt{}, validateErr
	}
	usedPath := path + ".used"
	// #nosec G304 -- usedPath is derived from the validated receipt path and run id.
	used, err := os.OpenFile(usedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ReviewReceipt{}, errors.New("review receipt already consumed")
		}
		return ReviewReceipt{}, fmt.Errorf("consume receipt: %w", err)
	}
	if _, writeErr := used.WriteString(now.UTC().Format(time.RFC3339Nano) + "\n"); writeErr != nil {
		_ = used.Close()
		_ = os.Remove(usedPath)
		return ReviewReceipt{}, fmt.Errorf("record receipt consumption: %w", writeErr)
	}
	if closeErr := used.Close(); closeErr != nil {
		_ = os.Remove(usedPath)
		return ReviewReceipt{}, fmt.Errorf("close receipt consumption: %w", closeErr)
	}
	return receipt, nil
}

func ValidateReviewReceipt(receipt ReviewReceipt, key []byte, expectedRunID, expectedDigest string, now time.Time) error {
	if len(key) == 0 {
		return errors.New("receipt signing key is empty")
	}
	if err := validateReceiptFields(receipt); err != nil {
		return err
	}
	if receipt.RunID != expectedRunID {
		return errors.New("receipt run_id does not match")
	}
	if receipt.DiffDigest != expectedDigest {
		return errors.New("receipt diff_digest does not match")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	created := receipt.CreatedAt.UTC()
	if created.After(now.UTC().Add(5 * time.Minute)) {
		return errors.New("receipt timestamp is in the future")
	}
	if now.UTC().Sub(created) > reviewReceiptMaxAge {
		return errors.New("receipt timestamp is stale")
	}
	provided, err := hex.DecodeString(receipt.Signature)
	if err != nil {
		return errors.New("receipt signature is not valid hex")
	}
	payload, err := json.Marshal(reviewReceiptPayloadFrom(receipt))
	if err != nil {
		return fmt.Errorf("marshal receipt payload: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return errors.New("receipt signature is invalid")
	}
	return nil
}

func decodeReviewReceipt(content []byte) (ReviewReceipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var receipt ReviewReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return ReviewReceipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReviewReceipt{}, errors.New("decode receipt: trailing JSON")
		}
		return ReviewReceipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	return receipt, nil
}

func validateReceiptFields(receipt ReviewReceipt) error {
	if !runstate.ValidRunID(receipt.RunID) {
		return fmt.Errorf("invalid receipt run_id %q", receipt.RunID)
	}
	if receipt.DiffDigest == "" {
		return errors.New("receipt diff_digest is empty")
	}
	switch receipt.Verdict {
	case "approve", "reject", "escalate":
	default:
		return fmt.Errorf("invalid receipt verdict %q", receipt.Verdict)
	}
	if receipt.CreatedAt.IsZero() {
		return errors.New("receipt created_at is required")
	}
	return nil
}

func reviewReceiptPayloadFrom(receipt ReviewReceipt) reviewReceiptPayload {
	reasons := receipt.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	return reviewReceiptPayload{
		RunID:      receipt.RunID,
		DiffDigest: receipt.DiffDigest,
		Verdict:    receipt.Verdict,
		Reasons:    reasons,
		CreatedAt:  receipt.CreatedAt.UTC(),
	}
}
