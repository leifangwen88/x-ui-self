package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func normalizePeerBaseURL(base string) string {
	base = strings.TrimSpace(base)
	base = strings.TrimRight(base, "/")
	return base
}

func postPeerEvent(peer SyncPeerConfig, secret string, evt SyncEventDTO) error {
	base := normalizePeerBaseURL(peer.BaseURL)
	if base == "" {
		return nil
	}
	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/panel-sync/event", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Panel-Sync-Secret", secret)
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func fetchPeerOutbox(peer SyncPeerConfig, since int64, localSecret string) ([]SyncEventDTO, error) {
	base := normalizePeerBaseURL(peer.BaseURL)
	if base == "" {
		return nil, nil
	}
	secret := strings.TrimSpace(peer.Secret)
	if secret == "" {
		secret = localSecret
	}
	url := base + "/api/panel-sync/outbox?since=" + strconv.FormatInt(since, 10)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Panel-Sync-Secret", secret)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var out struct {
		Success bool           `json:"success"`
		Obj     []SyncEventDTO `json:"obj"`
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, nil
	}
	return out.Obj, nil
}

func fetchPeerMembers(peer SyncPeerConfig, secret string) (*ClusterMembersResponse, error) {
	base := normalizePeerBaseURL(peer.BaseURL)
	if base == "" {
		return nil, fmt.Errorf("peer base url empty")
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/panel-sync/members", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Panel-Sync-Secret", secret)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("members http %d", resp.StatusCode)
	}
	var out struct {
		Success bool                   `json:"success"`
		Obj     *ClusterMembersResponse `json:"obj"`
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if !out.Success || out.Obj == nil {
		return nil, fmt.Errorf("invalid members response")
	}
	return out.Obj, nil
}

func fetchPeerSnapshot(peer SyncPeerConfig, secret string) (*PanelSyncSnapshot, error) {
	base := normalizePeerBaseURL(peer.BaseURL)
	if base == "" {
		return nil, fmt.Errorf("peer base url empty")
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/panel-sync/snapshot", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Panel-Sync-Secret", secret)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot http %d", resp.StatusCode)
	}
	var out struct {
		Success bool               `json:"success"`
		Obj     *PanelSyncSnapshot `json:"obj"`
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if !out.Success || out.Obj == nil {
		return nil, fmt.Errorf("invalid snapshot response")
	}
	return out.Obj, nil
}

func postPeerAlignApply(peer SyncPeerConfig, secret string, snap *PanelSyncSnapshot, alignedAt int64, originBaseURL string, scope string) error {
	base := normalizePeerBaseURL(peer.BaseURL)
	if base == "" {
		return fmt.Errorf("peer base url empty")
	}
	if strings.TrimSpace(scope) == "" {
		scope = AlignScopeFull
	}
	body, err := json.Marshal(PanelAlignRemoteApply{
		Snapshot:      snap,
		AlignedAt:     alignedAt,
		OriginBaseURL: normalizePeerBaseURL(originBaseURL),
		Scope:         scope,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/panel-sync/align-apply", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Panel-Sync-Secret", secret)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("align-apply http %d", resp.StatusCode)
	}
	return nil
}
