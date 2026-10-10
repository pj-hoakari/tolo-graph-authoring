# グラフ編集コンテキスト 入出力整理

ドメイン定義: graph_authoring_domain.md
デプロイ単位: graph_authoring_spec.md
対象外: 観測点グラフ紐づけの WebRTC 映像確認

## ドメインイベント↔RPC 対応

| ドメインイベント | 対応 RPC |
|---|---|
| GraphCreated | GraphAuthoringService.SaveGraph（初回の一括登録） |
| GraphSaved | GraphAuthoringService.SaveGraph |
| ObservationPointMappedToGraph | GraphAuthoringService.MapObservationPoint |
| QrLocationChanged | GraphAuthoringService.AddQrLocation／UpdateQrLocation／RemoveQrLocation |
| GraphRevisionPublished | GraphAuthoringService.PublishRevision |

## 他コンテキストとの接点

| 接点 | RPC |
|---|---|
| グラフの供給（観測が消費し Flow／Line へ受け渡し） | GraphSupplyService.GetCurrentRevision → `tolo.kernel.v1.Graph`（shared_kernel_context.md） |
| 観測点とグラフ要素の対応の供給 | GraphSupplyService.GetObservationPointMappings |
| 表示名の供給（ゲスト向け表示の組み立て用） | GraphSupplyService.GetDisplayNames（Guest Service が消費。ゲストコンテキスト） |
| 設計時ゲート指定の供給 | GraphSupplyService.GetGatePoints（観測が消費し GateState の初期構築に使用。局所行列誘導コンテキスト） |
| QR 設置箇所の供給 | GraphSupplyService.GetQrLocations（Guest Service が消費し発行 URL を生成。ゲストコンテキスト。観測も消費し QR 方式の観測点として読み替える。観測コンテキスト） |
| 管理 UI とのグラフ文書の受け渡し | GraphAuthoringService.SaveGraph（一括登録）／GetGraph（グラフ文書・紐づけ・QR 設置箇所・版情報） |
| イベント識別子の参照整合 | TenantService.GetEvent（テナントコンテキスト） |

供給系の応答の形は、公開済みのグラフ版から消費者ごとに最適化して導出する形を基本とする（編集都合のレイアウト・ラベルを最適化向けへ持ち込まない。レイアウトは版に含めず、下流へ供給しない）。共有カーネル Graph の供給を除き、具体形は実装フェーズで確定する。
