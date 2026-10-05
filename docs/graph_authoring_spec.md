# Graph Authoring 入出力仕様

package: `tolo.graph.v1`
実現するコンテキスト: graph_authoring_context.md（ドメイン定義 graph_authoring_domain.md）
役割: 会場グラフの正本保持。下流（観測経由で Flow／Line）は現在の版を参照・消費のみ

対象外: 観測点グラフ紐づけの WebRTC 映像確認（紐づけ登録の RPC のみ定義）

## 編集と供給の2文脈

本サービスはグラフを2つの文脈で扱う。

編集（Authoring）は、テナント側のユーザー（スタッフ／オーナー）が会場グラフを作成・管理する文脈。管理 UI のグラフエディタで編集が完結し、完成したグラフ文書（多言語ラベル・レイアウトを含む会場グラフの文書）を `SaveGraph` で一括登録する。ラベルやレイアウトを文書に含むのは管理用画面の表示・操作の都合である。本サービスはポイント・ルート単位の編集 RPC を持たない。

- グラフ文書は proto の `GraphDocument` 型の単一フィールドで運ぶ。文書スキーマの正本は本仕様の proto 定義であり、フロントとバックエンドは生成コードを通じて同じ構造を共有する
- `GraphDocument` はエディタ実装（React Flow 等）の写しではなく、ドメインの語彙（ノード・グループ・エッジ・ラベル・レイアウト）で定義する。エディタ内部表現との相互変換と編集専用データの除去はフロントの責務であり、サーバはエディタ実装の知識を持たない
- 文書スキーマの変更は proto の改版として明示的に行う。属性の追加は optional フィールドの追加（後方互換）、互換を壊す変更は package バージョンの改版で行う
- サーバは文書の意味を変えずに保存し、`GetGraph` で構造的に同一の文書を返す（エディタはこれから編集状態を再構築する。バイト単位の同一性は要求しない）
- サーバは保存時に構造検証を行い、公開時に共有カーネルの Graph へ導出する

供給（Supply）は、公開済みのグラフ版から消費者ごとに最適化した導出データを返す文脈。編集都合の情報（レイアウト・グループ）は下流へ持ち込まず、各消費者が必要とする射影だけを渡す。観測経由で Flow／Line へ渡る会場グラフは共有カーネルの Graph（ポイント・ルートの関係と構造属性のみ。ラベル・レイアウトを持たない）であり、観測点の配置は紐づけの供給で渡す。ゲスト向け表示（ラベル）の供給は、言語を指定してラベル付きのデータを返す形か、言語ラベルのみを返す形かを含めて応答形が未確定であり、実装フェーズで確定する。共有カーネル Graph の供給を除き、後述の供給系の定義はこの方針での暫定形である。

## RPC 一覧

### 編集（スタッフ／オーナーが設計時に使用）

| RPC | 説明（ユビキタス言語） | 認可 | 関連ドメインイベント |
|---|---|---|---|
| SaveGraph | グラフ文書を一括登録する。初回の登録がイベントへの会場グラフの新設（グラフはイベント単位）。保存は下書きを上書きし、公開までは下流に影響しない | `event_access` + events.manage | GraphCreated（初回）／GraphSaved |
| MapObservationPoint | 観測点をグラフ要素（ポイント／ルート）に対応づける。本コンテキストが紐づけを所有。同一グラフ要素へカメラ方式と QR 方式を併存させない（QR 設置箇所が参照する要素への紐づけは `failed_precondition`） | 同上 | ObservationPointMappedToGraph |
| AddQrLocation／UpdateQrLocation／RemoveQrLocation | QR 設置箇所（名称・種類・グラフ要素参照）の追加・編集・削除。掲示計画は会場設計の一部として本コンテキストが所有（URL の発行は Guest Service）。カメラ方式の観測点が紐づく要素への追加・付け替えは `failed_precondition` | 同上 | QrLocationChanged |
| GetGraph | 保存済みのグラフ文書・観測点紐づけ・QR 設置箇所・版情報を返す。管理 UI の表示とエディタへの復元用 | `event_access` + events.read | （参照のみ） |
| PublishRevision | 保存済みのグラフ文書をグラフ版として公開し、下流が消費可能にする | `event_access` + events.manage（公開は下流の消費対象を変える書き込み操作） | GraphRevisionPublished |

### 供給（下流向け）

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| GetCurrentRevision | 現在のグラフ版を共有カーネルの Graph として返す | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetObservationPointMappings | 観測点とグラフ要素の対応を返す（観測がスコア算出時に使用） | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetDisplayNames | ポイントのラベル（言語コード別）・ルート表示名・ルート端点を返す。ゲスト向け表示の組み立てに使用 | Guest Service | サービス間（token_use=service） | （参照のみ） |
| GetGatePoints | 設計時にゲート指定されたポイントの一覧を返す。観測が GateState の初期構築に使用 | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetQrLocations | QR 設置箇所の一覧（名称・種類・グラフ要素参照）を返す。Guest Service が発行 URL の生成に使用し、観測が QR 方式の観測点として読み替える | Guest Service、観測 | サービス間（token_use=service） | （参照のみ） |

## グラフ文書（GraphDocument）の構造

グラフ文書はノード（ポイントになる要素）・グループ（フロア等のレイアウト用まとまり）・エッジ（ルートになる要素）を持つ。定義は「参考 proto 定義」の `GraphDocument` が正本で、ここでは意味だけを述べる。

- ノード（`GraphNode`）: `node_id` はエディタ採番で文書内一意。公開後はそのまま `point_id` になる。`node_type` はポイント種別（目標／通過目標混在／通過専用／入退出点）。`labels` は言語コード → 表示名（例 `{"ja": "入口", "en": "Entrance"}`）。`group_id` は所属グループ（任意）
- グループ（`NodeGroup`）: レイアウト用のまとまり（フロア等）。`group_id` は文書内一意。グラフ構造へは影響しない。グループの入れ子は表現しない
- エッジ（`GraphEdge`）: `edge_id` はエディタ採番で文書内一意。公開後はそのまま `route_id` になる。端点はノードのみ（グループを端点にしない）。`direction` は一方通行（source→target）または両通行。`label` は表示名（任意）
- レイアウト（`Layout`）: エディタ上の位置と寸法。表示用途のみ

ゲート指定・容量ヒント・入退出点の有効無効・エッジ表示名の多言語化は、`GraphDocument` への optional フィールドの追加として今後行う（フィールドの定義は実装フェーズで確定する）。追加までの導出の扱いは「共有カーネル Graph への導出」に示す。

### 保存時の構造検証

`SaveGraph` は次を検証し、満たさない文書は `invalid_argument` で拒否する（型の整合は proto 層が担うため、ここでは値の検証だけを行う）。

- `node_id`・`group_id`・`edge_id` がそれぞれ文書内で一意
- `node_type`・`direction` が UNSPECIFIED でない
- エッジの `source_node_id`／`target_node_id` が文書内のノードを指す
- ノードの `group_id`（設定時）が文書内のグループを指す

## 共有カーネル Graph への導出

公開時（`PublishRevision`）に、保存済みのグラフ文書から共有カーネルの Graph を導出する。

| グラフ文書 | 共有カーネル Graph |
|---|---|
| `GraphNode` | Point（`point_id` = `node_id`） |
| `NODE_TYPE_GOAL`／`NODE_TYPE_GOAL_TRANSIT_MIXED`／`NODE_TYPE_TRANSIT_ONLY` | PointType の同名値。`is_boundary = false` |
| `NODE_TYPE_BOUNDARY` | `POINT_TYPE_TRANSIT_ONLY` ＋ `is_boundary = true`（入退出点は通過専用として扱う） |
| （入退出点の有効無効） | `boundary_active = true` で導出（有効無効のフィールドは今後追加。追加までは全入退出点を有効とみなす） |
| `NodeGroup`・`Layout`・`group_id` | 導出しない（レイアウト情報。グラフ構造へ影響させない） |
| `GraphEdge` | Route（`route_id` = `edge_id`、`source_node_id`→`from_point_id`、`target_node_id`→`to_point_id`） |
| `EDGE_DIRECTION_ONE_WAY`／`EDGE_DIRECTION_BOTH_WAYS` | `DIRECTION_ATTRIBUTE_ONE_WAY`／`DIRECTION_ATTRIBUTE_BOTH_WAYS` |
| （容量ヒント） | 未設定で導出（フィールドは今後追加） |

- ポイント・ルートの識別子はエディタ採番の文書内識別子をそのまま用い、下流（観測・Flow・Line・Guest Service）も同じ識別子で参照する。識別子はイベントの会場グラフに閉じており、サーバ採番の公開 ID（16 文字 hex）の発番規約の対象外
- 文書側の `NodeType`・`EdgeDirection` は編集の語彙、共有カーネルの `PointType`・`DirectionAttribute` は供給の語彙として別に定義する（文書スキーマの進化を共有カーネルから独立させる）。対応は上表が正本
- 表示名の正本はノードの `labels`（言語コード別）とエッジの `label`。共有カーネルの Graph には含めず、GetDisplayNames で供給する
- 設計時のゲート指定の正本は本コンテキスト（文書のフィールドとして今後追加）。共有カーネルの Graph には含めず（カーネルにゲートを持ち込まない不変条件の維持）、GetGatePoints で観測へ別途供給する。フィールドが追加されるまで GetGatePoints は空の一覧を返す

## 参考 proto 定義

```proto
syntax = "proto3";
package tolo.graph.v1;
import "tolo/kernel/v1/kernel.proto";

service GraphAuthoringService {
  rpc SaveGraph(SaveGraphRequest) returns (GraphMeta);
  rpc MapObservationPoint(MapObservationPointRequest) returns (ObservationPointMapping);
  rpc AddQrLocation(AddQrLocationRequest) returns (QrLocation);
  rpc UpdateQrLocation(UpdateQrLocationRequest) returns (QrLocation);
  rpc RemoveQrLocation(RemoveQrLocationRequest) returns (RemoveResponse);
  rpc GetGraph(GetGraphRequest) returns (GetGraphResponse);
  rpc PublishRevision(PublishRevisionRequest) returns (GraphMeta);
}

// 供給系: 公開版から消費者ごとに最適化した導出データを返す
// 共有カーネル Graph の供給（GetCurrentRevision）を除き、応答形は暫定（実装フェーズで確定）
service GraphSupplyService {
  rpc GetCurrentRevision(GetCurrentRevisionRequest) returns (tolo.kernel.v1.Graph);
  rpc GetObservationPointMappings(GetMappingsRequest) returns (GetMappingsResponse);
  rpc GetDisplayNames(GetDisplayNamesRequest) returns (GetDisplayNamesResponse);
  rpc GetGatePoints(GetGatePointsRequest) returns (GetGatePointsResponse);
  rpc GetQrLocations(GetQrLocationsRequest) returns (GetQrLocationsResponse);
}

message GraphMeta {
  string event_id = 1;
  string revision_id = 2;        // 公開済みの現在版
  string draft_revision_id = 3;  // 編集中の版（あれば）
}

// グラフ文書: エディタで完成させた会場グラフの文書（構造・多言語ラベル・レイアウト）
// スキーマの正本は本定義。エディタ内部表現（React Flow 等）との相互変換はフロントの責務
// 属性の追加は optional フィールドの追加として明示的に行う
message GraphDocument {
  repeated GraphNode nodes = 1;   // ポイントになる要素
  repeated NodeGroup groups = 2;  // レイアウト用グループ（フロア等。グラフ構造へは影響しない）
  repeated GraphEdge edges = 3;   // ルートになる要素
}

// ノード: 公開時に共有カーネルの Point へ導出される
message GraphNode {
  string node_id = 1;              // エディタ採番。文書内で一意。公開後はそのまま point_id
  NodeType node_type = 2;
  map<string, string> labels = 3;  // 言語コード（"ja"・"en" 等）→ 表示名
  string group_id = 4;             // 所属グループ（任意。NodeGroup.group_id を指す）
  Layout layout = 5;
}

enum NodeType {
  NODE_TYPE_UNSPECIFIED = 0;
  NODE_TYPE_GOAL = 1;                // 目標
  NODE_TYPE_GOAL_TRANSIT_MIXED = 2;  // 通過目標混在
  NODE_TYPE_TRANSIT_ONLY = 3;        // 通過専用
  NODE_TYPE_BOUNDARY = 4;            // 入退出点（導出時は通過専用＋is_boundary）
}

// レイアウト用グループ: フロア等のまとまり。入れ子は表現しない
message NodeGroup {
  string group_id = 1;             // エディタ採番。文書内で一意
  map<string, string> labels = 2;  // 言語コード → 表示名
  Layout layout = 3;
}

// エディタ上の配置（表示用途のみ。導出対象外）
message Layout {
  double x = 1;
  double y = 2;
  optional double width = 3;
  optional double height = 4;
}

// エッジ: 公開時に共有カーネルの Route へ導出される
message GraphEdge {
  string edge_id = 1;              // エディタ採番。文書内で一意。公開後はそのまま route_id
  string source_node_id = 2;       // 端点はノードのみ（グループを端点にしない）
  string target_node_id = 3;
  EdgeDirection direction = 4;
  optional string label = 5;       // 表示名（任意。多言語化はフィールド追加として今後行う）
}

enum EdgeDirection {
  EDGE_DIRECTION_UNSPECIFIED = 0;
  EDGE_DIRECTION_ONE_WAY = 1;    // 一方通行（source→target）
  EDGE_DIRECTION_BOTH_WAYS = 2;  // 両通行
}

// グラフ文書の一括登録。初回の登録がグラフの新設
message SaveGraphRequest {
  string event_id = 1;        // 存在しないイベント ID へは作らない（テナントで参照整合）
  GraphDocument document = 2;
}
message RemoveResponse {}

// 観測点グラフ紐づけ（WebRTC での映像確認手順は対象外）
// 同一のポイント／ルートに複数の観測点を紐づけられる。スコア算出は要素単位（anchor.route_position は表示用途のみ）
message ObservationPointMapping {
  string observation_point_id = 1;
  tolo.kernel.v1.GraphAnchor anchor = 2;
}
message MapObservationPointRequest {
  string event_id = 1;
  ObservationPointMapping mapping = 2;  // 存在するポイント／ルートに対してのみ
}

// QR 設置箇所: 掲示計画（会場設計の一部）として本コンテキストが所有。URL の発行は Guest Service
// 同一のポイント／ルートに複数置ける。ルート途中は anchor.route_position（表示用途のみ）
message QrLocation {
  string qr_location_id = 1;
  string name = 2;
  string kind = 3;  // 種類（Guest Service 側の種類→ページ構成の対応に使う選択キー）
  tolo.kernel.v1.GraphAnchor anchor = 4;  // 存在するポイント／ルートに対してのみ
}
message AddQrLocationRequest {
  string event_id = 1;
  QrLocation qr_location = 2;  // qr_location_id は未指定（サーバ採番）
}
message UpdateQrLocationRequest {
  string event_id = 1;
  QrLocation qr_location = 2;
}
message RemoveQrLocationRequest {
  string event_id = 1;
  string qr_location_id = 2;
}
message GetQrLocationsRequest {
  string event_id = 1;
}
message GetQrLocationsResponse {
  repeated QrLocation qr_locations = 1;
}

// 管理 UI の表示とエディタへの復元用（下流供給は GetCurrentRevision＝kernel 型）
message GetGraphRequest {
  string event_id = 1;
}
message GetGraphResponse {
  GraphMeta meta = 1;
  GraphDocument document = 2;  // 保存した文書と構造的に同一の文書を返す
  repeated ObservationPointMapping observation_point_mappings = 3;
  repeated QrLocation qr_locations = 4;
}

message PublishRevisionRequest {
  string event_id = 1;
}

message GetCurrentRevisionRequest {
  string event_id = 1;
}
message GetMappingsRequest {
  string event_id = 1;
}
message GetMappingsResponse {
  string revision_id = 1;
  repeated ObservationPointMapping mappings = 2;
}

// 表示名の供給（ゲスト向け表示の組み立て用。正本はグラフ文書のラベル）
// 応答形は暫定（言語指定でラベル付きデータを返す形か、言語ラベルのみを返す形かを含め実装フェーズで確定）
message GetDisplayNamesRequest {
  string event_id = 1;
}
message GetDisplayNamesResponse {
  string revision_id = 1;
  map<string, LocalizedText> display_names = 2;     // point_id → ラベル（言語コード別。未設定のポイントは含まれない）
  map<string, string> route_display_names = 3;      // route_id → 表示名（未設定は含まれない）
  map<string, RouteEndpoints> route_endpoints = 4;  // route_id → 端点（表示名未設定ルートの合成用）
}
message LocalizedText {
  map<string, string> texts = 1;  // 言語コード（"ja"・"en" 等）→ 表示文字列
}
message RouteEndpoints {
  string from_point_id = 1;
  string to_point_id = 2;
}

// 設計時ゲート指定の供給（正本は本コンテキスト。共有カーネルの Graph には含めない）
message GetGatePointsRequest {
  string event_id = 1;
}
message GetGatePointsResponse {
  string revision_id = 1;
  repeated string gate_point_ids = 2;
}
```

## 補足

- 保存は下書きの上書きとして累積し、`PublishRevision` までは下流に影響しない
- 参照整合は `failed_precondition`: 存在しないイベント ID へのグラフ新設（初回保存）、存在しないポイント／ルートへの紐づけ（観測点・QR 設置箇所とも）、既存の観測点紐づけ・QR 設置箇所が参照するポイント／ルートを文書から消す保存（先に紐づけ側を削除してから保存し直す）
  同一のポイント／ルートへカメラ方式の観測点紐づけと QR 設置箇所を併存させる操作も `failed_precondition` とする。同じ来場者をカメラと QR で二重に数えないためで、MapObservationPoint と AddQrLocation／UpdateQrLocation の双方で検証する
- QR 設置箇所（掲示計画）の正本は本コンテキスト。管理 UI がグラフと同じ画面で管理でき、Guest Service は GetQrLocations で pull して発行 URL（設置箇所×種類）を生成する
  観測も GetQrLocations で取得し、QR 設置箇所を QR 方式の観測点として読み替える（観測点として別途登録しない。Observation）
- 配置系（観測点紐づけ・QR 設置箇所）は同一のポイント／ルートに複数置ける（各配置が独立の識別子を持つ）。ただしカメラ方式の観測点紐づけと QR 設置箇所は同一要素に併存させない
  ルート途中の位置は `GraphAnchor.route_position`（from→to の比率 0〜1）で表し、表示用途のみに使う（最適化・スコア算出には使わない）
- ゲート地点もポイントとして設計時にオーサリングする（開閉は運用時の操作であり、観測が OperateGate で直接受ける。Observation）
- ルートの表示名（`GraphEdge.label`）は任意。未設定のルートはゲスト側が端点ポイント名から合成する（`route_endpoints` を併せて供給）
- グラフ版はグラフ文書全体のスナップショット
  PublishRevision で全体を1版として公開し、下流は常に整合した現在版を消費する。公開済み版の文書と導出結果は変更しない。差分供給は将来の最適化
  版の同一性は文書の内容で判定する。`draft_revision_id` は保存した文書の内容から導出し（同じ文書は同じ ID）、`SaveGraph` は同じ文書の再保存に対して同じ `GraphMeta` を返す（冪等）
  `PublishRevision` は公開時点の `draft_revision_id` をそのまま `revision_id` とする。`draft_revision_id` と `revision_id` が異なるとき、公開版に対して未公開の編集がある（編集中）とみなす。編集後に公開版と同じ内容へ戻した文書は編集中とみなさない
  導出は文書の直列化に依存し、構造的に同一でも要素の順序が異なる文書は別の ID になる。構造の正規化は構造検証の導入時に行う。スナップショットの保存形は DB スキーマとあわせて確定する
  同時編集の検出（後勝ちの防止）は本判定の対象外とする
- イベントアーカイブ時は会場グラフ・紐づけを保持し読み取り専用化する
  編集系 RPC は `failed_precondition`、供給系 RPC（GetCurrentRevision 等）は継続。復元（EventUnarchived）で編集を再開できる
