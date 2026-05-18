package service

const (
	AlignSourceLocal = "local"
	AlignSourcePeer  = "peer"
	AlignSourceMerge = "merge"
	AlignScopeFull   = "full"
)

// PanelSyncSnapshot 用于首次对齐：游戏、SOCKS、入站、IP 标记（不含面板登录与 secret）
type PanelSyncSnapshot struct {
	NodeId             string                `json:"nodeId"`
	ExportedAt         int64                 `json:"exportedAt"`
	Games              []GameBackup          `json:"games"`
	SocksProxies       []SocksBackup         `json:"socksProxies"`
	Inbounds           []InboundBackup       `json:"inbounds"`
	SocksGameMarks     []SocksGameMarkBackup `json:"socksGameMarks"`
	XrayTemplateConfig string                `json:"xrayTemplateConfig"`
}

type PanelAlignCounts struct {
	Games        int `json:"games"`
	Socks        int `json:"socks"`
	Inbounds     int `json:"inbounds"`
	Marks        int `json:"marks"`
	XrayTemplate int `json:"xrayTemplate"`
}

type PanelAlignCategoryDiff struct {
	LocalOnly int `json:"localOnly"`
	PeerOnly  int `json:"peerOnly"`
	Conflict  int `json:"conflict"`
}

type PanelAlignDiff struct {
	Games          PanelAlignCategoryDiff `json:"games"`
	Socks          PanelAlignCategoryDiff `json:"socks"`
	Inbounds       PanelAlignCategoryDiff `json:"inbounds"`
	InboundRemarks PanelAlignCategoryDiff `json:"inboundRemarks"`
	Marks          PanelAlignCategoryDiff `json:"marks"`
	XrayTemplate   PanelAlignCategoryDiff `json:"xrayTemplate"`
}

type PanelAlignCompareResult struct {
	PeerKey       string           `json:"peerKey"`
	PeerName      string           `json:"peerName"`
	PeerReachable bool             `json:"peerReachable"`
	PeerError     string           `json:"peerError,omitempty"`
	AlignedAt     int64            `json:"alignedAt"`
	Local         PanelAlignCounts   `json:"local"`
	Peer          PanelAlignCounts   `json:"peer"`
	Diff          PanelAlignDiff     `json:"diff"`
}

type PanelAlignApplyRequest struct {
	PeerIndex            int    `json:"peerIndex"`
	GamesSource          string `json:"gamesSource"`
	SocksSource          string `json:"socksSource"`
	InboundsSource       string `json:"inboundsSource"`
	InboundRemarkSource  string `json:"inboundRemarkSource"`
	MarksSource          string `json:"marksSource"`
	XrayTemplateSource   string `json:"xrayTemplateSource"`
	PushToPeer           bool   `json:"pushToPeer"`
}

type PanelAlignRemoteApply struct {
	Snapshot      *PanelSyncSnapshot `json:"snapshot"`
	AlignedAt     int64              `json:"alignedAt"`
	OriginBaseURL string             `json:"originBaseURL"`
	Scope         string             `json:"scope"`
}

type PanelAlignApplyResult struct {
	AlignedAt   int64  `json:"alignedAt"`
	PeerPushed  bool   `json:"peerPushed"`
	PeerPushErr string `json:"peerPushErr,omitempty"`
}

type PanelPeerAlignStatus struct {
	PeerKey   string `json:"peerKey"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	AlignedAt int64  `json:"alignedAt"`
}
