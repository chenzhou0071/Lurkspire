// spawnpoints.go — 出生/复活点（用户场景实测空地；高层点 = 建筑顶平台）
package room

import "math/rand"

// SpawnPoint 出生点（世界坐标——y 支持高层平台）
type SpawnPoint struct{ X, Y, Z float32 }

// SpawnPoints 地图出生点列表（用户实地选点）
var SpawnPoints = []SpawnPoint{
	{284, 0, 55},
	{273, 0, 165},
	{188, 32, 96}, // 高层
	{191, 0, 126},
	{111, 0, 63},
	{143, 0, 165},
	{194, 0, 76},
	{254, 0, 85},
	{143, 16.5, 92},  // 高层
	{262, 12.5, 135}, // 高层
}

// PickSpawn 随机挑一个出生点（加入/复活时防出生贴脸）
func PickSpawn() SpawnPoint {
	return SpawnPoints[rand.Intn(len(SpawnPoints))]
}
