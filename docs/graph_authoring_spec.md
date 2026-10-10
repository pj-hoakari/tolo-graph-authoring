# Graph Authoring 入出力仕様

package: `tolo.graph.v1`
実現するコンテキスト: graph_authoring_context.md（ドメイン定義 graph_authoring_domain.md）
役割: グラフの正本を保持する。下流（観測経由で Flow／Line）は現在の版を参照・消費するだけである

対象外: 観測点グラフ紐づけの WebRTC 映像確認（紐づけ登録の RPC のみ定義）

## 編集と供給の2文脈

本サービスはグラフを2つの文脈で扱う。

編集（Authoring）は、テナント側のユーザー（スタッフ／オーナー）がグラフを作成・管理する文脈。編集は管理 UI のグラフエディタの中で完結し、完成したグラフ文書（多言語ラベル・レイアウトを含むグラフの文書）を `SaveGraph` で一括登録する。ラベルやレイアウトを文書に含むのは管理用画面の表示・操作の都合である。本サービスはポイント・ルート単位の編集 RPC を持たない。

- グラフ文書は proto の `GraphDocument` 型の単一フィールドで受け渡す。文書スキーマの正本は本仕様の proto 定義であり、フロントとバックエンドは生成コードを通じて同じ構造を共有する
- `GraphDocument` はエディタ実装（React Flow 等）の写しではなく、ドメインの語彙（ノード・グループ・エッジ・ラベル・レイアウト）で定義する。エディタ内部表現との相互変換と編集専用データの除去はフロントの責務であり、サーバはエディタ実装の知識を持たない
- 文書スキーマの変更は proto の改版として明示的に行う。属性の追加は optional フィールドの追加（後方互換）、互換を壊す変更は package バージョンの改版で行う
- サーバは文書の意味を変えずに保存し、`GetGraph` で構造的に同一の文書を返す（エディタはこれから編集状態を再構築する。バイト単位の同一性は要求しない。要素は種類ごとに ID 順に並べて返す）
- サーバは保存時に構造検証を行い、公開時に共有カーネルの Graph へ導出する

供給（Supply）は、公開済みのグラフ版から消費者ごとに最適化した導出データを返す文脈。編集都合の情報（レイアウト・グループ・外部ポイント）は下流へ持ち込まず、各消費者が必要とする射影だけを渡す。観測経由で Flow／Line へ渡るグラフは共有カーネルの Graph（ポイント・ルートの関係と構造属性のみ。ラベル・レイアウトを持たない）であり、観測点の配置は紐づけの供給で渡す。ゲスト向け表示（ラベル）の供給は、言語を指定してラベル付きのデータを返す形か、言語ラベルのみを返す形かを含めて応答形が未確定であり、実装フェーズで確定する。共有カーネル Graph の供給を除き、後述の供給系の定義はこの方針での暫定形である。

## RPC 一覧

### 編集（スタッフ／オーナーが設計時に使用）

| RPC | 説明（ユビキタス言語） | 認可 | 関連ドメインイベント |
|---|---|---|---|
| SaveGraph | グラフ文書を一括登録する。初回の登録がイベントへのグラフの新設（グラフはイベント単位）。保存は下書き（カーネルとラベル）とレイアウトを上書きし、公開までは下流に影響しない | `event_access` + events.manage | GraphCreated（初回）／GraphSaved |
| MapObservationPoint | 観測点をグラフ要素（ポイント／ルート）に対応づける。本コンテキストが紐づけを所有。同一グラフ要素へカメラ方式と QR 方式を併存させない（QR 設置箇所が参照する要素への紐づけは `failed_precondition`） | 同上 | ObservationPointMappedToGraph |
| AddQrLocation／UpdateQrLocation／RemoveQrLocation | QR 設置箇所（名称・種類・グラフ要素参照）の追加・編集・削除。掲示計画は会場設計の一部として本コンテキストが所有（URL の発行は Guest Service）。カメラ方式の観測点が紐づく要素への追加・付け替えは `failed_precondition` | 同上 | QrLocationChanged |
| GetGraph | 保存済みのグラフ文書・観測点紐づけ・QR 設置箇所・版情報を返す。管理 UI の表示とエディタへの復元用。アーカイブ中のイベントでも読める。グラフを保存していないイベントは `not_found` | `event_access` + events.read | （参照のみ） |
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

グラフ文書はノード（ポイントになる要素と外部ポイント）・グループ（フロア等のレイアウト用まとまり）・エッジ（ルートになる要素と外部ポイントとの接続）を持つ。定義は「参考 proto 定義」の `GraphDocument` が正本で、ここでは意味だけを述べる。

- ノード（`GraphNode`）: `node_id` はエディタ採番で文書内一意。公開後はそのまま `point_id` になる。`node_type` はポイント種別（目標／通過目標混在／通過専用）か外部ポイントを表す。`labels` は言語コード → 表示名（例 `{"ja": "入口", "en": "Entrance"}`）。`group_id` は所属グループ（任意）
- 外部ポイント（`node_type` が `NODE_TYPE_EXTERNAL` のノード）: 会場の外を表す仮想的な点。入退出点を示すために置き、共有カーネルへは導出しない。エッジを短く引けるよう複数置いてよい。グループに属さず、外部ポイント同士をエッジで結ばない
- 入退出点: 外部ポイントとエッジで結ばれた、外部ポイント以外のノード。ノード種別は自身のものを保つ。向きは外部ポイントとのエッジから決まる。外部ポイントからの一方通行だけなら入場、外部ポイントへの一方通行だけなら退場、両通行のエッジを持つか入場と退場の両方の接続を持つなら入退場とする
- グループ（`NodeGroup`）: レイアウト用のまとまり（フロア等）。`group_id` は文書内一意。`parent_group_id` で親グループを指して入れ子にできる（任意）。`min_width`／`min_height` は、エディタが子要素に合わせて決める寸法の下限（任意）。グラフ構造へは影響しない
- エッジ（`GraphEdge`）: `edge_id` はエディタ採番で文書内一意。公開後はそのまま `route_id` になる。端点はノードのみ（グループを端点にしない）。`direction` は一方通行（source→target）または両通行。`label` は表示名（任意）。外部ポイントを端点に持つエッジは入退出点の向きを表すだけで、ルートにならない
- レイアウト（`Layout`）: エディタ上の位置と寸法。表示用途のみ。ポイントと外部ポイントは中心の座標、グループは左上の座標で表し、グループに属する要素の座標は親グループからの相対座標とする。サーバは送られた値をそのまま保存し、座標の意味を解釈しない。ゲスト向けの地図に使う会場の実際の位置は、エディタのレイアウトとは別に扱う

ゲート指定・容量ヒント・入退出点の有効無効・エッジ表示名の多言語化は、`GraphDocument` への optional フィールドの追加として今後行う（フィールドの定義は実装フェーズで確定する）。追加までの導出の扱いは「共有カーネル Graph への導出」に示す。

### 保存時の構造検証

`SaveGraph` は次を検証し、満たさない文書は `invalid_argument` で拒否する（型の整合は proto 層が担うため、ここでは値の検証だけを行う）。

- `node_id`・`group_id`・`edge_id` がそれぞれ文書内で一意
- `node_type`・`direction` が UNSPECIFIED でない
- エッジの `source_node_id`／`target_node_id` が文書内のノードを指す
- ノードの `group_id`（設定時）が文書内のグループを指す
- グループの `parent_group_id`（設定時）が文書内のグループを指し、親をたどって循環しない
- 外部ポイントが `group_id` を持たない
- 外部ポイント同士を結ぶエッジがない
- レイアウトの座標・寸法と `min_width`／`min_height` が有限の値である（NaN と無限大を含まない）

## 共有カーネル Graph への導出

公開時（`PublishRevision`）に、保存済みのグラフ文書から共有カーネルの Graph を導出する。

| グラフ文書 | 共有カーネル Graph |
|---|---|
| `GraphNode`（外部ポイントを除く） | Point（`point_id` = `node_id`） |
| `NODE_TYPE_GOAL`／`NODE_TYPE_GOAL_TRANSIT_MIXED`／`NODE_TYPE_TRANSIT_ONLY` | PointType の同名値 |
| 外部ポイントと結ばれたノード（入退出点） | `boundary` を設定する。外部ポイントと結ばれていないポイントは `boundary` を持たない |
| 入退出点の向き（入場／退場／入退場） | `boundary.direction` の `BOUNDARY_DIRECTION_ENTRY`／`BOUNDARY_DIRECTION_EXIT`／`BOUNDARY_DIRECTION_ENTRY_AND_EXIT` |
| （入退出点の有効無効） | `boundary.active = true` で導出（有効無効のフィールドは今後追加。追加までは全入退出点を有効とみなす） |
| `NODE_TYPE_EXTERNAL` のノード・外部ポイントを端点に持つエッジ | 導出しない（入退出点の向きにだけ反映する） |
| `NodeGroup`・`Layout`・`group_id` | 導出しない（レイアウト情報。グラフ構造へ影響させない） |
| `GraphEdge`（外部ポイントを端点に持たないもの） | Route（`route_id` = `edge_id`、`source_node_id`→`from_point_id`、`target_node_id`→`to_point_id`） |
| `EDGE_DIRECTION_ONE_WAY`／`EDGE_DIRECTION_BOTH_WAYS` | `DIRECTION_ATTRIBUTE_ONE_WAY`／`DIRECTION_ATTRIBUTE_BOTH_WAYS` |
| （容量ヒント） | 未設定で導出（フィールドは今後追加） |

- ポイント・ルートの識別子はエディタ採番の文書内識別子をそのまま用い、下流（観測・Flow・Line・Guest Service）も同じ識別子で参照する。識別子はイベントのグラフに閉じており、サーバ採番の公開 ID（16 文字 hex）の発番規約の対象外
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

// グラフ文書: エディタで完成させたグラフの文書（構造・多言語ラベル・レイアウト）
// スキーマの正本は本定義。エディタ内部表現（React Flow 等）との相互変換はフロントの責務
// 属性の追加は optional フィールドの追加として明示的に行う
message GraphDocument {
  repeated GraphNode nodes = 1;   // ポイントになる要素と外部ポイント
  repeated NodeGroup groups = 2;  // レイアウト用グループ（フロア等。グラフ構造へは影響しない）
  repeated GraphEdge edges = 3;   // ルートになる要素と外部ポイントとの接続
}

// ノード: 外部ポイント以外は公開時に共有カーネルの Point へ導出される
message GraphNode {
  string node_id = 1;              // エディタ採番。文書内で一意。公開後はそのまま point_id
  NodeType node_type = 2;
  map<string, string> labels = 3;  // 言語コード（"ja"・"en" 等）→ 表示名
  string group_id = 4;             // 所属グループ（任意。NodeGroup.group_id を指す。外部ポイントは持たない）
  Layout layout = 5;               // 中心の座標。グループに属するときは親グループからの相対座標
}

enum NodeType {
  NODE_TYPE_UNSPECIFIED = 0;
  NODE_TYPE_GOAL = 1;                // 目標
  NODE_TYPE_GOAL_TRANSIT_MIXED = 2;  // 通過目標混在
  NODE_TYPE_TRANSIT_ONLY = 3;        // 通過専用
  NODE_TYPE_EXTERNAL = 4;            // 外部ポイント（会場の外。結ばれたノードが入退出点になる。導出しない）
}

// レイアウト用グループ: フロア等のまとまり。入れ子にできる
message NodeGroup {
  string group_id = 1;               // エディタ採番。文書内で一意
  map<string, string> labels = 2;    // 言語コード → 表示名
  Layout layout = 3;                 // 左上の座標。親グループがあるときは親からの相対座標
  string parent_group_id = 4;        // 親グループ（任意。NodeGroup.group_id を指す。循環させない）
  optional double min_width = 5;     // エディタが子要素に合わせて決める幅の下限
  optional double min_height = 6;    // エディタが子要素に合わせて決める高さの下限
}

// エディタ上の配置（表示用途のみ。導出対象外。サーバは値を解釈せずに保存する）
message Layout {
  double x = 1;
  double y = 2;
  optional double width = 3;
  optional double height = 4;
}

// エッジ: 外部ポイントを端点に持たないものは公開時に共有カーネルの Route へ導出される
// 外部ポイントを端点に持つエッジは入退出点の向きを表す（外部ポイント同士は結ばない）
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
  ObservationPointMapping mapping = 2;  // 存在するポイント／ルートに対してのみ（外部ポイントとその接続は不可）
}

// QR 設置箇所: 掲示計画（会場設計の一部）として本コンテキストが所有。URL の発行は Guest Service
// 同一のポイント／ルートに複数置ける。ルート途中は anchor.route_position（表示用途のみ）
message QrLocation {
  string qr_location_id = 1;
  string name = 2;
  string kind = 3;  // 種類（Guest Service 側の種類→ページ構成の対応に使う選択キー）
  tolo.kernel.v1.GraphAnchor anchor = 4;  // 存在するポイント／ルートに対してのみ（外部ポイントとその接続は不可）
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
  GraphDocument document = 2;  // 保存した文書と構造的に同一の文書を返す（要素は種類ごとに ID 順）
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
- 参照整合は `failed_precondition`: 存在しないイベント ID へのグラフ新設（初回保存）、存在しないポイント／ルートへの紐づけ（観測点・QR 設置箇所とも。外部ポイントと、外部ポイントを端点に持つエッジはポイント／ルートではないため、紐づけ先にできない）、既存の観測点紐づけ・QR 設置箇所が参照するポイント／ルートを文書から消す保存（先に紐づけ側を削除してから保存し直す）
  同一のポイント／ルートへカメラ方式の観測点紐づけと QR 設置箇所を併存させる操作も `failed_precondition` とする。同じ来場者をカメラと QR で二重に数えないためで、MapObservationPoint と AddQrLocation／UpdateQrLocation の双方で検証する
- QR 設置箇所（掲示計画）の正本は本コンテキスト。管理 UI がグラフと同じ画面で管理でき、Guest Service は GetQrLocations で pull して発行 URL（設置箇所×種類）を生成する
  観測も GetQrLocations で取得し、QR 設置箇所を QR 方式の観測点として読み替える（観測点として別途登録しない。Observation）
- 配置系（観測点紐づけ・QR 設置箇所）は同一のポイント／ルートに複数置ける（各配置が独立の識別子を持つ）。ただしカメラ方式の観測点紐づけと QR 設置箇所は同一要素に併存させない
  ルート途中の位置は `GraphAnchor.route_position`（from→to の比率 0〜1）で表し、表示用途のみに使う（最適化・スコア算出には使わない）
- ゲート地点もポイントとして設計時にオーサリングする（開閉は運用時の操作であり、観測が OperateGate で直接受ける。Observation）
- ルートの表示名（`GraphEdge.label`）は任意。未設定のルートはゲスト側が端点ポイント名から合成する（`route_endpoints` を併せて供給）
- 保存した文書は3つの部分に分けて持つ
  カーネルは入退出点の向きを含むポイントとルート、ラベルはポイント・グループ・ルートの表示名である
  レイアウトは座標と寸法、グループへの所属、グループの親と最小寸法、外部ポイント（表示名と座標を含む）、外部ポイントを端点に持つエッジ（向きと表示名を含む）である
- グラフ版はカーネルとラベルのスナップショット
  版は、外部へ出る可能性があり、ある程度の頻度で変わる内容を管理する。レイアウトは管理 UI の表示にだけ使い、外部へ出ないため版に含めない
  PublishRevision でカーネルとラベルの全体を1版として公開し、下流は常に整合した現在版を消費する。公開済み版の内容と導出結果は変更しない。差分供給は将来の最適化
  レイアウトはイベントのグラフごとに版の外へ1つだけ保存し、保存のたびに上書きする。版管理せず、下流へも供給しない
- 版の同一性はカーネルとラベルの内容で判定する
  `draft_revision_id` は、カーネルとラベルの要素を ID 順に並べて直列化した内容の SHA-256 の先頭 8 バイトを hex で表したものである。同じ内容は同じ ID になり、要素の順序は ID に影響しない。`SaveGraph` は同じ文書の再保存に対して同じ `GraphMeta` を返す（冪等）
  レイアウトだけの変更は ID を変えない。要素の移動・寸法の変更・入れ子の付け替え、外部ポイントとのエッジを向きの同じ別の外部ポイントへ付け替えること、そのエッジの表示名の変更がこれに当たる。外部ポイントとの接続の追加・削除と向きの変更は、入退出点の向きが変わるため ID を変える
  `PublishRevision` は公開時点の `draft_revision_id` をそのまま `revision_id` とする。`draft_revision_id` と `revision_id` が異なるとき、公開版に対して未公開の編集がある（編集中）とみなす。編集後に公開版と同じ内容へ戻した文書と、レイアウトだけを変えた文書は編集中とみなさない
  同時編集の検出（後勝ちの防止）は本判定の対象外とする
- イベントアーカイブ時はグラフ・紐づけを保持し、読み取り専用にする
  編集系 RPC は `failed_precondition`、供給系 RPC（GetCurrentRevision 等）と GetGraph は継続。復元（EventUnarchived）で編集を再開できる
