WITH roster_players AS (
    SELECT
        id AS player_id,
        row_number() OVER (ORDER BY id) AS player_number
    FROM players
    WHERE id BETWEEN '00000000-0000-0000-0000-000000000011'::uuid
        AND '00000000-0000-0000-0000-000000000050'::uuid
), recent_days AS (
    SELECT day_offset
    FROM generate_series(0, 4) AS days(day_offset)
)
INSERT INTO training_records(id, player_id, training_date, content, reflection, version, created_at, updated_at)
SELECT
    ('00000000-0000-0000-0001-' || lpad((player_number * 10 + day_offset)::text, 12, '0'))::uuid,
    player_id,
    current_date - day_offset::integer,
    CASE day_offset
        WHEN 0 THEN '完成挥棒节奏与击球点练习，共五组。'
        WHEN 1 THEN '完成短距离冲刺、折返跑与核心力量训练。'
        WHEN 2 THEN '完成接传球基本功和不同方向守备移动练习。'
        WHEN 3 THEN '完成长距离传球与肩部稳定性训练。'
        ELSE '完成动态热身、恢复跑和全身拉伸。'
    END,
    CASE day_offset
        WHEN 0 THEN '击球节奏稳定，后两组的动作一致性较好。'
        WHEN 1 THEN '启动速度有所提升，最后一组体能下降较明显。'
        WHEN 2 THEN '脚步衔接顺畅，反手方向仍需增加重复次数。'
        WHEN 3 THEN '传球落点基本稳定，注意保持肩部放松。'
        ELSE '身体状态恢复良好，没有明显不适。'
    END,
    1,
    now() - day_offset * interval '1 day',
    now() - day_offset * interval '1 day'
FROM roster_players
CROSS JOIN recent_days
ORDER BY player_number, day_offset;
