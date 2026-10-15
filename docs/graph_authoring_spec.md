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
- サーバは保存時に構造検証を行い、公開時に公開の条件を検証して共有カーネルの Graph へ導出する

供給（Supply）は、公開済みのグラフ版から消費者ごとに最適化した導出データを返す文脈。編集都合の情報（レイアウト・グループ・外部ポイント）は下流へ持ち込まず、各消費者が必要とする射影だけを渡す。観測経由で Flow／Line へ渡るグラフは共有カーネルの Graph（ポイント・ルートの関係と構造属性のみ。ラベル・レイアウトを持たない）であり、観測点の配置は紐づけの供給で渡す。ゲスト向け表示（ラベル）の供給は、言語を指定してラベル付きのデータを返す形か、言語ラベルのみを返す形かを含めて応答形が未確定であり、実装フェーズで確定する。共有カーネル Graph の供給を除き、後述の供給系の定義はこの方針での暫定形である。

## RPC 一覧

### 編集（スタッフ／オーナーが設計時に使用）

| RPC | 説明（ユビキタス言語） | 認可 | 関連ドメインイベント |
|---|---|---|---|
| SaveGraph | グラフ文書を一括登録する。初回の登録がイベントへのグラフの新設（グラフはイベント単位）。保存は下書き（カーネルとラベル）とレイアウトを上書きし、公開までは下流に影響しない | `event_access` + events.manage | GraphCreated（初回）／GraphSaved |
| MapObservationPoint | 観測点をグラフ要素（ポイント／ルート）に対応づける。本コンテキストが紐づけを所有。同一グラフ要素へカメラ方式と QR 方式を併存させない（QR 設置箇所が参照する要素への紐づけは `failed_precondition`）。紐づけは下書きの配置（観測点紐づけ・QR 設置箇所）を書き換え、公開までは下流に影響しない。応答は紐づけと更新後の `GraphMeta` を返す | 同上 | ObservationPointMappedToGraph |
| AddQrLocation／UpdateQrLocation／RemoveQrLocation | QR 設置箇所（名称・種類・グラフ要素参照）の追加・編集・削除。掲示計画は会場設計の一部として本コンテキストが所有（URL の発行は Guest Service）。カメラ方式の観測点が紐づく要素への追加・付け替えは `failed_precondition`。下書きの配置を書き換え、公開までは下流に影響しない。応答は更新後の `GraphMeta` を含む | 同上 | QrLocationChanged |
| GetGraph | 保存済みのグラフ文書と、下書きの観測点紐づけ・QR 設置箇所、版情報を返す。管理 UI の表示とエディタへの復元用。アーカイブ中のイベントでも読める。グラフを保存していないイベントは `not_found` | `event_access` + events.read | （参照のみ） |
| PublishRevision | 保存済みのグラフ文書と下書きの配置をグラフ版として公開し、下流が消費可能にする。公開の条件を満たさない下書きは `failed_precondition`（「公開時の検証」） | `event_access` + events.manage（公開は下流の消費対象を変える書き込み操作） | GraphRevisionPublished |

### 供給（下流向け）

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| GetCurrentRevision | 現在のグラフ版を共有カーネルの Graph として返す | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetObservationPointMappings | 指定した版の観測点とグラフ要素の対応を返す（観測がスコア算出時に使用）。観測は GetCurrentRevision で得た `revision_id` を指定し、グラフと紐づけを同じ版からそろえる | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetDisplayNames | ポイントのラベル（言語コード別）・ルート表示名・ルート端点を返す。ゲスト向け表示の組み立てに使用 | Guest Service | サービス間（token_use=service） | （参照のみ） |
| GetGatePoints | 設計時にゲート指定されたポイントの一覧を返す。観測が GateState の初期構築に使用 | 観測 | サービス間（token_use=service） | （参照のみ） |
| GetQrLocations | 指定した版の QR 設置箇所の一覧（名称・種類・グラフ要素参照）を返す。Guest Service が発行 URL の生成に使用し、観測が QR 方式の観測点として読み替える | Guest Service、観測 | サービス間（token_use=service） | （参照のみ） |

GetObservationPointMappings と GetQrLocations は `revision_id` を必須で受け取り、その版の配置を返す。公開済みの版であれば、現在の版でなくても返す。公開済みの版にない `revision_id` は `not_found`、`revision_id` の未指定は `invalid_argument` とする。

GetQrLocations は `tenant_id` も必須で受け取る。本サービスは、グラフの保存時に Tenant Management の `GetEvent` で確かめたイベントの所属テナントと比べ、一致しなければ `permission_denied` を返す。`tenant_id` の未指定は `invalid_argument` とする。呼び出し元はマシン起点の `token_use=service` でテナントのクレームを持たないため、受け取った `tenant_id` を本サービスが持つ所属で確かめる。

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

- `node_id`・`group_id`・`edge_id` が空でなく、それぞれ文書内で一意
- `node_type`・`direction` が UNSPECIFIED でない
- エッジの `source_node_id`／`target_node_id` が文書内のノードを指す
- ノードの `group_id`（設定時）が文書内のグループを指す
- グループの `parent_group_id`（設定時）が文書内のグループを指し、親をたどって循環しない
- 外部ポイントが `group_id` を持たない
- 外部ポイント同士を結ぶエッジがない
- エッジの `source_node_id` と `target_node_id` が同じノードを指さない
- 外部ポイント以外のノードが、外部ポイント以外のノードと結ぶエッジ（ルートになるエッジ）を1本以上持つ。外部ポイントとだけ結ばれたノードは満たさない
- 外部ポイントがエッジを1本以上持つ
- グループが、所属するノードか子グループを1つ以上持つ
- レイアウトの座標・寸法と `min_width`／`min_height` が有限の値である（NaN と無限大を含まない）

ルートを持たないポイント、どこにも結ばれない外部ポイント、要素のないグループ、自分自身へのエッジは、公開後に Flow Control や Operation など下流の中核の動作に影響する。このため、編集途中の下書きであっても保存時に拒否する。

#### 検証エラーの詳細

検証は最初の違反で止めず、文書内のすべての違反を集める。`invalid_argument` のエラーには、集めた違反を `GraphViolations` として Connect のエラー詳細に載せる。違反の並び順は契約に含めない。エラーのメッセージ文は人が読むためのもので、利用側は種類と ID をエラー詳細から読む。

ID が空か重複する要素は、以降の検査の対象にしない（重複する ID は最初の要素だけを検査する）。文書内にないノードを端点に持つエッジも、以降の検査の対象にしない。種類や向きが UNSPECIFIED の要素、レイアウトが有限でない要素は、以降の検査を続ける。対象から外した要素が原因で別の要素に生じた違反（外したエッジしか持たないノードのルートの欠如など）も、そのまま違反として返す。

| 検査 | `kind` | `element_kind` | `element_id` | `related_ids` |
|---|---|---|---|---|
| `node_id` が空 | `EMPTY_NODE_ID` | `NODE` | 空文字列 | なし |
| `node_id` が重複 | `DUPLICATE_NODE_ID` | `NODE` | 重複した ID | なし |
| `node_type` が UNSPECIFIED | `NODE_WITHOUT_TYPE` | `NODE` | ノード | なし |
| ノードのレイアウトが有限でない | `NON_FINITE_NODE_LAYOUT` | `NODE` | ノード | なし |
| `group_id` が空 | `EMPTY_GROUP_ID` | `GROUP` | 空文字列 | なし |
| `group_id` が重複 | `DUPLICATE_GROUP_ID` | `GROUP` | 重複した ID | なし |
| グループのレイアウトか `min_width`／`min_height` が有限でない | `NON_FINITE_GROUP_LAYOUT` | `GROUP` | グループ | なし |
| 外部ポイントが `group_id` を持つ | `EXTERNAL_NODE_IN_GROUP` | `NODE` | 外部ポイント | 所属先のグループ |
| ノードの `group_id` が文書内のグループを指さない | `MISSING_GROUP` | `NODE` | ノード | 指しているグループ ID |
| グループの `parent_group_id` が文書内のグループを指さない | `MISSING_PARENT_GROUP` | `GROUP` | グループ | 指している親グループ ID |
| 親をたどると循環に入る | `GROUP_CYCLE` | `GROUP` | 循環に入るグループ（循環の下にある子孫を含む） | なし |
| `edge_id` が空 | `EMPTY_EDGE_ID` | `EDGE` | 空文字列 | なし |
| `edge_id` が重複 | `DUPLICATE_EDGE_ID` | `EDGE` | 重複した ID | なし |
| `direction` が UNSPECIFIED | `EDGE_WITHOUT_DIRECTION` | `EDGE` | エッジ | なし |
| 端点が文書内のノードを指さない | `MISSING_NODE` | `EDGE` | エッジ | 指しているノード ID のうち、文書内にないもの（source、target の順） |
| 外部ポイント同士を結ぶ | `EDGE_BETWEEN_EXTERNAL_NODES` | `EDGE` | エッジ | なし |
| 端点が同じノード | `SELF_LOOP_EDGE` | `EDGE` | エッジ | 端点のノード |
| 外部ポイントがエッジを持たない | `EXTERNAL_NODE_WITHOUT_EDGE` | `NODE` | 外部ポイント | なし |
| 外部ポイント以外のノードがルートになるエッジを持たない | `NODE_WITHOUT_ROUTE` | `NODE` | ノード | なし |
| グループが所属するノードも子グループも持たない | `GROUP_WITHOUT_MEMBERS` | `GROUP` | グループ | なし |

`kind` の列は `VIOLATION_KIND_`、`element_kind` の列は `ELEMENT_KIND_` を省いて書く。

### 公開時の検証

`PublishRevision` は、保存済みの下書きの文書が次の条件を満たすかを検証し、満たさない下書きは公開せずに `failed_precondition` で拒否する。下書きの文書は保存時の構造検証を通っているため、ここでは公開の条件だけを検査する。

- ポイントを1件以上持つ。ポイントは、外部ポイント以外のノード（共有カーネルの Point へ導出するノード）を指す
- 各ポイントの `labels` が、空文字列でない表示名を1つ以上持つ

ポイントのない版を公開すると、下流はポイントもルートもない空のグラフを受け取る。エラーにはならず、推定の対象がない版が現在の版になる。どの言語にも表示名のないポイントは GetDisplayNames に含まれない。ゲスト向けの表示では、その地点の名前を出せず、表示名を持たないルートのうちそのポイントを端点に持つものの名前も合成できない。どちらも編集途中の下書きでは普通に起きる状態なので、保存時ではなく公開時に拒否する。

次の状態は公開を拒否しない。

- 特定の言語の表示名がないポイント
- 観測点紐づけも QR 設置箇所もない版
- 観測点紐づけも QR 設置箇所も持たないポイント・ルート
- グループの `labels` とエッジの `label` が空であること

検証は最初の違反で止めず、すべての違反を集める。`failed_precondition` のエラーには、集めた違反を `GraphViolations` として Connect のエラー詳細に載せる。並び順とメッセージ文の扱いは「検証エラーの詳細」と同じとする。

| 検査 | `kind` | `element_kind` | `element_id` | `related_ids` |
|---|---|---|---|---|
| ポイントが1件もない | `GRAPH_WITHOUT_POINT` | `UNSPECIFIED` | 空文字列 | なし |
| ポイントの `labels` に空文字列でない表示名がない | `POINT_WITHOUT_LABEL` | `NODE` | ポイント | なし |

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
  rpc MapObservationPoint(MapObservationPointRequest) returns (MapObservationPointResponse);
  rpc AddQrLocation(AddQrLocationRequest) returns (AddQrLocationResponse);
  rpc UpdateQrLocation(UpdateQrLocationRequest) returns (UpdateQrLocationResponse);
  rpc RemoveQrLocation(RemoveQrLocationRequest) returns (RemoveQrLocationResponse);
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
  string draft_revision_id = 3;  // 下書きの版（文書と配置から導出。配置の編集でも変わる）
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

// グラフ文書の検証違反。保存時は invalid_argument、公開時は failed_precondition のエラー詳細に GraphViolations として載せる
message GraphViolations {
  repeated GraphViolation violations = 1;  // 検出したすべての違反
}

message GraphViolation {
  ViolationKind kind = 1;
  ElementKind element_kind = 2;   // 違反した要素の種類（要素に結びつかない違反では UNSPECIFIED）
  string element_id = 3;          // 違反した要素の ID（ID が空の違反と要素に結びつかない違反では空文字列）
  repeated string related_ids = 4;  // 違反に関わる他の要素の ID（対応は「検証エラーの詳細」の表）
}

// 違反の種類。公開時の拒否などの違反は値の追加として足す
enum ViolationKind {
  VIOLATION_KIND_UNSPECIFIED = 0;
  VIOLATION_KIND_EMPTY_NODE_ID = 1;
  VIOLATION_KIND_DUPLICATE_NODE_ID = 2;
  VIOLATION_KIND_NODE_WITHOUT_TYPE = 3;
  VIOLATION_KIND_NON_FINITE_NODE_LAYOUT = 4;
  VIOLATION_KIND_EMPTY_GROUP_ID = 5;
  VIOLATION_KIND_DUPLICATE_GROUP_ID = 6;
  VIOLATION_KIND_NON_FINITE_GROUP_LAYOUT = 7;
  VIOLATION_KIND_EXTERNAL_NODE_IN_GROUP = 8;
  VIOLATION_KIND_MISSING_GROUP = 9;
  VIOLATION_KIND_MISSING_PARENT_GROUP = 10;
  VIOLATION_KIND_GROUP_CYCLE = 11;
  VIOLATION_KIND_EMPTY_EDGE_ID = 12;
  VIOLATION_KIND_DUPLICATE_EDGE_ID = 13;
  VIOLATION_KIND_EDGE_WITHOUT_DIRECTION = 14;
  VIOLATION_KIND_MISSING_NODE = 15;
  VIOLATION_KIND_EDGE_BETWEEN_EXTERNAL_NODES = 16;
  VIOLATION_KIND_SELF_LOOP_EDGE = 17;
  VIOLATION_KIND_EXTERNAL_NODE_WITHOUT_EDGE = 18;
  VIOLATION_KIND_NODE_WITHOUT_ROUTE = 19;
  VIOLATION_KIND_GROUP_WITHOUT_MEMBERS = 20;
  VIOLATION_KIND_GRAPH_WITHOUT_POINT = 21;
  VIOLATION_KIND_POINT_WITHOUT_LABEL = 22;
}

enum ElementKind {
  ELEMENT_KIND_UNSPECIFIED = 0;
  ELEMENT_KIND_NODE = 1;
  ELEMENT_KIND_GROUP = 2;
  ELEMENT_KIND_EDGE = 3;
}

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
message MapObservationPointResponse {
  ObservationPointMapping mapping = 1;
  GraphMeta meta = 2;  // 紐づけ後の下書きの版を含む
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
message AddQrLocationResponse {
  QrLocation qr_location = 1;
  GraphMeta meta = 2;  // 追加後の下書きの版を含む
}
message UpdateQrLocationRequest {
  string event_id = 1;
  QrLocation qr_location = 2;
}
message UpdateQrLocationResponse {
  QrLocation qr_location = 1;
  GraphMeta meta = 2;  // 編集後の下書きの版を含む
}
message RemoveQrLocationRequest {
  string event_id = 1;
  string qr_location_id = 2;
}
message RemoveQrLocationResponse {
  GraphMeta meta = 1;  // 削除後の下書きの版を含む
}
message GetQrLocationsRequest {
  string event_id = 1;
  string revision_id = 2;  // 必須。公開済みの版（現在の版でなくてよい）
  string tenant_id = 3;    // 必須。イベントの所属テナントと一致しなければ permission_denied
}
message GetQrLocationsResponse {
  repeated QrLocation qr_locations = 1;
  string revision_id = 2;
}

// 管理 UI の表示とエディタへの復元用（下流供給は GetCurrentRevision＝kernel 型）
message GetGraphRequest {
  string event_id = 1;
}
message GetGraphResponse {
  GraphMeta meta = 1;
  GraphDocument document = 2;  // 保存した文書と構造的に同一の文書を返す（要素は種類ごとに ID 順）
  repeated ObservationPointMapping observation_point_mappings = 3;  // 下書きの紐づけ
  repeated QrLocation qr_locations = 4;                              // 下書きの QR 設置箇所
}

message PublishRevisionRequest {
  string event_id = 1;
}

message GetCurrentRevisionRequest {
  string event_id = 1;
}
message GetMappingsRequest {
  string event_id = 1;
  string revision_id = 2;  // 必須。公開済みの版（現在の版でなくてよい）
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

- 保存と配置の編集は下書きの上書きとして累積し、`PublishRevision` までは下流に影響しない
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
- グラフ版はカーネル・ラベル・配置（観測点紐づけ・QR 設置箇所）のスナップショット
  版は、外部へ出る可能性があり、ある程度の頻度で変わる内容を管理する。配置は観測と Guest Service へ供給するため版に含める。レイアウトは管理 UI の表示にだけ使い、外部へ出ないため版に含めない
  PublishRevision で、下書きの文書（カーネルとラベル）と配置を同じトランザクションで写し、その全体を1版として公開する。下流は常に整合した現在版を消費する。公開済み版の内容（配置を含む）と導出結果は変更しない。差分供給は将来の最適化
  供給系の配置は公開済みの版から返し、`GetGraph` は下書きの配置を返す
  レイアウトはイベントのグラフごとに版の外へ1つだけ保存し、保存のたびに上書きする。版管理せず、下流へも供給しない
- 版の同一性はカーネル・ラベル・配置の内容で判定する。配置は識別子まで含めて比べるため、QR 設置箇所を消して同じ内容で足し直すと ID が変わる（`qr_location_id` はサーバがランダムに採番する）
  `draft_revision_id` は2段で求める。まず、カーネルとラベルの要素を ID 順に並べて直列化した内容のハッシュ（文書のハッシュ）を求め、下書きとともに保存する。次に、文書のハッシュと、配置を ID 順に並べて直列化した内容の SHA-256 を求め、その先頭 8 バイトを hex で表したものを ID とする。同じ内容は同じ ID になり、要素と配置の順序は ID に影響しない。`SaveGraph` は同じ文書の再保存に対して同じ `GraphMeta` を返す（冪等）
  配置の編集は、保存してある文書のハッシュと編集後の配置から `draft_revision_id` を求め直し、応答の `GraphMeta` で返す
  レイアウトだけの変更は ID を変えない。要素の移動・寸法の変更・入れ子の付け替え、外部ポイントとのエッジを向きの同じ別の外部ポイントへ付け替えること、そのエッジの表示名の変更がこれに当たる。外部ポイントとの接続の追加・削除と向きの変更は、入退出点の向きが変わるため ID を変える
  `PublishRevision` は公開時点の `draft_revision_id` をそのまま `revision_id` とする。`draft_revision_id` と `revision_id` が異なるとき、公開版に対して未公開の編集がある（編集中）とみなす。配置だけを変えた下書きも編集中とみなす。編集後に公開版と同じ内容へ戻した下書きと、レイアウトだけを変えた下書きは編集中とみなさない
  同時編集の検出（後勝ちの防止）は本判定の対象外とする
- イベントアーカイブ時はグラフ・紐づけを保持し、読み取り専用にする
  編集系 RPC は `failed_precondition`、供給系 RPC（GetCurrentRevision 等）と GetGraph は継続。復元（EventUnarchived）で編集を再開できる
