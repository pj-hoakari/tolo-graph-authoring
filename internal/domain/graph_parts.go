package domain

type GraphParts struct {
	Kernel GraphKernel
	Labels GraphLabels
	Layout GraphLayout
}

type GraphKernel struct {
	Points []Point `json:"points"`
	Routes []Route `json:"routes"`
}

type GraphLabels struct {
	Points map[string]map[string]string `json:"points"`
	Groups map[string]map[string]string `json:"groups"`
	Routes map[string]*string           `json:"routes"`
}

type GraphLayout struct {
	Points        map[string]PointLayout `json:"points"`
	Externals     []ExternalNode         `json:"externals"`
	Groups        []GroupLayout          `json:"groups"`
	ExternalEdges []Edge                 `json:"external_edges"`
}

type PointLayout struct {
	GroupID string `json:"group_id"`
	Layout  Layout `json:"layout"`
}

type ExternalNode struct {
	ID     string            `json:"id"`
	Labels map[string]string `json:"labels"`
	Layout Layout            `json:"layout"`
}

type GroupLayout struct {
	ID            string   `json:"id"`
	ParentGroupID string   `json:"parent_group_id"`
	MinWidth      *float64 `json:"min_width"`
	MinHeight     *float64 `json:"min_height"`
	Layout        Layout   `json:"layout"`
}

var pointTypes = map[NodeType]PointType{
	NodeTypeGoal:             PointTypeGoal,
	NodeTypeGoalTransitMixed: PointTypeGoalTransitMixed,
	NodeTypeTransitOnly:      PointTypeTransitOnly,
}

var nodeTypes = map[PointType]NodeType{
	PointTypeGoal:             NodeTypeGoal,
	PointTypeGoalTransitMixed: NodeTypeGoalTransitMixed,
	PointTypeTransitOnly:      NodeTypeTransitOnly,
}

var directionAttributes = map[EdgeDirection]DirectionAttribute{
	EdgeDirectionOneWay:   DirectionAttributeOneWay,
	EdgeDirectionBothWays: DirectionAttributeBothWays,
}

var edgeDirections = map[DirectionAttribute]EdgeDirection{
	DirectionAttributeOneWay:   EdgeDirectionOneWay,
	DirectionAttributeBothWays: EdgeDirectionBothWays,
}

func (d GraphDocument) Parts() GraphParts {
	d = d.canonical()

	external := make(map[string]bool)
	for _, node := range d.Nodes {
		external[node.ID] = node.Type == NodeTypeExternal
	}

	boundary := make(map[string]BoundaryDirection)

	for _, edge := range d.Edges {
		inward, outward := BoundaryDirectionEntry, BoundaryDirectionExit
		if edge.Direction == EdgeDirectionBothWays {
			inward, outward = BoundaryDirectionEntryAndExit, BoundaryDirectionEntryAndExit
		}

		if external[edge.SourceNodeID] {
			boundary[edge.TargetNodeID] |= inward
		}

		if external[edge.TargetNodeID] {
			boundary[edge.SourceNodeID] |= outward
		}
	}

	parts := GraphParts{
		Kernel: GraphKernel{Points: []Point{}, Routes: []Route{}},
		Labels: GraphLabels{
			Points: make(map[string]map[string]string),
			Groups: make(map[string]map[string]string),
			Routes: make(map[string]*string),
		},
		Layout: GraphLayout{
			Points:        make(map[string]PointLayout),
			Externals:     []ExternalNode{},
			Groups:        []GroupLayout{},
			ExternalEdges: []Edge{},
		},
	}

	for _, node := range d.Nodes {
		if external[node.ID] {
			parts.Layout.Externals = append(parts.Layout.Externals, ExternalNode{ID: node.ID, Labels: node.Labels, Layout: node.Layout})

			continue
		}

		parts.Kernel.Points = append(parts.Kernel.Points, Point{
			ID:                node.ID,
			Type:              pointTypes[node.Type],
			BoundaryDirection: boundary[node.ID],
			BoundaryActive:    true,
		})
		parts.Labels.Points[node.ID] = node.Labels
		parts.Layout.Points[node.ID] = PointLayout{GroupID: node.GroupID, Layout: node.Layout}
	}

	for _, group := range d.Groups {
		parts.Labels.Groups[group.ID] = group.Labels
		parts.Layout.Groups = append(parts.Layout.Groups, GroupLayout{
			ID:            group.ID,
			ParentGroupID: group.ParentGroupID,
			MinWidth:      group.MinWidth,
			MinHeight:     group.MinHeight,
			Layout:        group.Layout,
		})
	}

	for _, edge := range d.Edges {
		if external[edge.SourceNodeID] || external[edge.TargetNodeID] {
			parts.Layout.ExternalEdges = append(parts.Layout.ExternalEdges, edge)

			continue
		}

		parts.Kernel.Routes = append(parts.Kernel.Routes, Route{
			ID:          edge.ID,
			FromPointID: edge.SourceNodeID,
			ToPointID:   edge.TargetNodeID,
			Direction:   directionAttributes[edge.Direction],
		})
		parts.Labels.Routes[edge.ID] = edge.Label
	}

	return parts
}

func (p GraphParts) Document() GraphDocument {
	document := GraphDocument{
		Nodes:  make([]Node, 0, len(p.Kernel.Points)+len(p.Layout.Externals)),
		Groups: make([]Group, 0, len(p.Layout.Groups)),
		Edges:  make([]Edge, 0, len(p.Kernel.Routes)+len(p.Layout.ExternalEdges)),
	}

	for _, point := range p.Kernel.Points {
		layout := p.Layout.Points[point.ID]
		document.Nodes = append(document.Nodes, Node{
			ID:      point.ID,
			Type:    nodeTypes[point.Type],
			Labels:  p.Labels.Points[point.ID],
			GroupID: layout.GroupID,
			Layout:  layout.Layout,
		})
	}

	for _, node := range p.Layout.Externals {
		document.Nodes = append(document.Nodes, Node{
			ID:      node.ID,
			Type:    NodeTypeExternal,
			Labels:  node.Labels,
			GroupID: "",
			Layout:  node.Layout,
		})
	}

	for _, group := range p.Layout.Groups {
		document.Groups = append(document.Groups, Group{
			ID:            group.ID,
			Labels:        p.Labels.Groups[group.ID],
			ParentGroupID: group.ParentGroupID,
			MinWidth:      group.MinWidth,
			MinHeight:     group.MinHeight,
			Layout:        group.Layout,
		})
	}

	for _, route := range p.Kernel.Routes {
		document.Edges = append(document.Edges, Edge{
			ID:           route.ID,
			SourceNodeID: route.FromPointID,
			TargetNodeID: route.ToPointID,
			Direction:    edgeDirections[route.Direction],
			Label:        p.Labels.Routes[route.ID],
		})
	}

	document.Edges = append(document.Edges, p.Layout.ExternalEdges...)

	return document.canonical()
}
