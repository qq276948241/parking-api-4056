-- ========================================
-- 停车场管理系统 数据库建表脚本
-- PostgreSQL 12+
-- ========================================

-- 启用UUID扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ========================================
-- 1. 管理员表
-- ========================================
CREATE TABLE IF NOT EXISTS admins (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    real_name VARCHAR(50),
    role VARCHAR(20) NOT NULL DEFAULT 'admin', -- super_admin / admin
    parking_lot_id UUID, -- 关联停车场ID（超管为空）
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ========================================
-- 2. 停车场表
-- ========================================
CREATE TABLE IF NOT EXISTS parking_lots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    address TEXT,
    contact_phone VARCHAR(20),
    total_spaces INT NOT NULL DEFAULT 0,
    hourly_rate DECIMAL(10,2) NOT NULL DEFAULT 5.00,
    daily_max DECIMAL(10,2) NOT NULL DEFAULT 50.00,
    free_minutes INT NOT NULL DEFAULT 30,
    fee_tiers JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE parking_lots ADD COLUMN IF NOT EXISTS fee_tiers JSONB;

-- ========================================
-- 3. 车位表
-- ========================================
CREATE TABLE IF NOT EXISTS parking_spaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parking_lot_id UUID NOT NULL REFERENCES parking_lots(id) ON DELETE CASCADE,
    space_number VARCHAR(20) NOT NULL,
    zone VARCHAR(50),
    type VARCHAR(20) NOT NULL DEFAULT 'standard', -- standard / reserved / disabled
    status VARCHAR(20) NOT NULL DEFAULT 'available', -- available / occupied / reserved / maintenance
    vehicle_plate VARCHAR(20),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(parking_lot_id, space_number)
);

CREATE INDEX IF NOT EXISTS idx_parking_spaces_lot ON parking_spaces(parking_lot_id);
CREATE INDEX IF NOT EXISTS idx_parking_spaces_status ON parking_spaces(status);

-- ========================================
-- 4. 停车记录表
-- ========================================
CREATE TABLE IF NOT EXISTS parking_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parking_lot_id UUID NOT NULL REFERENCES parking_lots(id) ON DELETE CASCADE,
    space_id UUID REFERENCES parking_spaces(id) ON DELETE SET NULL,
    vehicle_plate VARCHAR(20) NOT NULL,
    vehicle_type VARCHAR(20) NOT NULL DEFAULT 'car', -- car / suv / truck / motorcycle
    entry_time TIMESTAMP NOT NULL,
    exit_time TIMESTAMP,
    duration_minutes INT DEFAULT 0,
    hourly_rate DECIMAL(10,2) NOT NULL DEFAULT 0,
    discount DECIMAL(10,2) NOT NULL DEFAULT 0,
    total_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    paid_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    payment_status VARCHAR(20) NOT NULL DEFAULT 'unpaid', -- unpaid / paid / partial / waived
    payment_method VARCHAR(20), -- cash / wechat / alipay / card / monthly
    monthly_card_id UUID,
    is_monthly BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'parking', -- parking / completed
    remarks TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_parking_records_lot ON parking_records(parking_lot_id);
CREATE INDEX IF NOT EXISTS idx_parking_records_plate ON parking_records(vehicle_plate);
CREATE INDEX IF NOT EXISTS idx_parking_records_status ON parking_records(status);
CREATE INDEX IF NOT EXISTS idx_parking_records_entry ON parking_records(entry_time);

-- ========================================
-- 5. 月卡表
-- ========================================
CREATE TABLE IF NOT EXISTS monthly_cards (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parking_lot_id UUID NOT NULL REFERENCES parking_lots(id) ON DELETE CASCADE,
    card_number VARCHAR(50) NOT NULL UNIQUE,
    vehicle_plate VARCHAR(20) NOT NULL,
    owner_name VARCHAR(50),
    owner_phone VARCHAR(20),
    plan_name VARCHAR(50) NOT NULL,
    plan_type VARCHAR(20) NOT NULL DEFAULT 'monthly', -- monthly / quarterly / yearly
    price DECIMAL(10,2) NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active / expired / suspended
    paid_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    payment_method VARCHAR(20),
    remarks TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_monthly_cards_lot ON monthly_cards(parking_lot_id);
CREATE INDEX IF NOT EXISTS idx_monthly_cards_plate ON monthly_cards(vehicle_plate);
CREATE INDEX IF NOT EXISTS idx_monthly_cards_status ON monthly_cards(status);
CREATE INDEX IF NOT EXISTS idx_monthly_cards_end ON monthly_cards(end_date);

-- ========================================
-- 6. 缴费记录表
-- ========================================
CREATE TABLE IF NOT EXISTS payment_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parking_lot_id UUID NOT NULL REFERENCES parking_lots(id) ON DELETE CASCADE,
    parking_record_id UUID REFERENCES parking_records(id) ON DELETE SET NULL,
    monthly_card_id UUID REFERENCES monthly_cards(id) ON DELETE SET NULL,
    payment_type VARCHAR(20) NOT NULL, -- parking / monthly
    amount DECIMAL(10,2) NOT NULL,
    payment_method VARCHAR(20) NOT NULL, -- cash / wechat / alipay / card
    transaction_no VARCHAR(100),
    operator_id UUID REFERENCES admins(id),
    status VARCHAR(20) NOT NULL DEFAULT 'success', -- success / failed / refunded
    remarks TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payment_records_lot ON payment_records(parking_lot_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_created ON payment_records(created_at);

-- ========================================
-- 触发器：自动更新 updated_at
-- ========================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_admins_updated_at ON admins;
CREATE TRIGGER update_admins_updated_at BEFORE UPDATE ON admins
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_parking_lots_updated_at ON parking_lots;
CREATE TRIGGER update_parking_lots_updated_at BEFORE UPDATE ON parking_lots
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_parking_spaces_updated_at ON parking_spaces;
CREATE TRIGGER update_parking_spaces_updated_at BEFORE UPDATE ON parking_spaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_parking_records_updated_at ON parking_records;
CREATE TRIGGER update_parking_records_updated_at BEFORE UPDATE ON parking_records
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_monthly_cards_updated_at ON monthly_cards;
CREATE TRIGGER update_monthly_cards_updated_at BEFORE UPDATE ON monthly_cards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ========================================
-- 初始化数据
-- ========================================

-- 默认超级管理员: admin / admin123 (密码已bcrypt哈希)
INSERT INTO admins (username, password_hash, real_name, role, is_active)
VALUES (
    'admin',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    '超级管理员',
    'super_admin',
    TRUE
) ON CONFLICT (username) DO NOTHING;
