package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTenantID 鏄崟绉熸埛閮ㄧ讲鐨勯粯璁ょ鎴?ID銆?// 涓?internal/api/auth.go銆乮nternal/session/manager.go 涓畾涔夌殑甯搁噺淇濇寔涓€鑷达紝
// 娉ㄥ唽涓庝細璇濊惤搴撳潎纭紪鐮佸紩鐢ㄨ ID銆?const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// EnsureDefaultTenant 骞傜瓑纭繚榛樿绉熸埛瀛樺湪銆?//
// users/sessions 绛夎〃鍧囨湁 tenants(id) 澶栭敭锛坲sers_tenant_id_fkey 绛夛級锛?// 鑰?tenants 琛ㄦ湰韬病鏈変换浣曠瀛愭暟鎹紱鑻ラ粯璁ょ鎴风己澶憋紝娉ㄥ唽浼氱洿鎺ヨ繚鍙?// 澶栭敭绾︽潫锛圫QLSTATE 23503锛夈€傝鍑芥暟涓嶄緷璧栬縼绉荤姸鎬侊紙schema_migrations锛夛紝
// 瀵归€氳繃浠讳綍鏂瑰紡鍒濆鍖栫殑搴撻兘鑳界敓鏁堛€?func EnsureDefaultTenant(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name) VALUES ($1, 'default') ON CONFLICT (id) DO NOTHING`,
		DefaultTenantID)
	return err
}
