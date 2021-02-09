package log

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/jinzhu/gorm"
	"github.com/lib/pq"
	"github.com/pkg/errors"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/postgres"
	"github.com/smartcontractkit/chainlink/core/store/models"
)

//go:generate mockery --name ORM --output ./mocks/ --case=underscore --structname ORM --filename orm.go

type ORM interface {
	UpsertLog(log types.Log) error
	UpsertBroadcastForListener(log types.Log, jobID *models.ID, jobIDV2 int32) error
	UpsertBroadcastsForListenerSinceBlock(blockNumber uint64, address common.Address, jobID *models.ID, jobIDV2 int32) error
	WasBroadcastConsumed(blockHash common.Hash, logIndex uint, jobID *models.ID, jobIDV2 int32) (bool, error)
	MarkBroadcastConsumed(blockHash common.Hash, logIndex uint, jobID *models.ID, jobIDV2 int32) error
	UnconsumedLogsPriorToBlock(blockNumber uint64) ([]types.Log, error)
	DeleteLogAndBroadcasts(blockHash common.Hash, logIndex uint) error
	DeleteUnconsumedBroadcastsForListener(jobID *models.ID, jobIDV2 int32) error
}

type orm struct {
	db *gorm.DB
}

var _ ORM = (*orm)(nil)

func NewORM(db *gorm.DB) *orm {
	return &orm{db}
}

func (o *orm) UpsertLog(log types.Log) error {
	topics := make([][]byte, len(log.Topics))
	for i, topic := range log.Topics {
		x := make([]byte, len(topic))
		copy(x, topic[:])
		topics[i] = x
	}
	err := o.db.Exec(`
        INSERT INTO eth_logs (block_hash, block_number, index, address, topics, data, created_at) VALUES ($1, $2, $3, $4, $5, $6, NOW())
        ON CONFLICT (block_hash, index) DO UPDATE SET (
            block_hash,
            block_number,
            index,
            address,
            topics,
            data
        ) = (
            EXCLUDED.block_hash,
            EXCLUDED.block_number,
            EXCLUDED.index,
            EXCLUDED.address,
            EXCLUDED.topics,
            EXCLUDED.data
        )
    `, log.BlockHash, log.BlockNumber, log.Index, log.Address, pq.ByteaArray(topics), log.Data).Error
	return err
}

func (o *orm) UpsertBroadcastForListener(log types.Log, jobID *models.ID, jobIDV2 int32) error {
	return o.upsertBroadcastForListener(o.db, log, jobID, jobIDV2)
}

func (o *orm) UpsertBroadcastsForListenerSinceBlock(blockNumber uint64, address common.Address, jobID *models.ID, jobIDV2 int32) error {
	ctx := context.TODO() // TODO: change this once our gormv2 migration lands
	return postgres.GormTransaction(ctx, o.db, func(tx *gorm.DB) error {
		logs, err := FetchLogs(tx, `SELECT * FROM eth_logs WHERE eth_logs.block_number >= ? AND address = ?`, blockNumber, address)
		if err != nil {
			return err
		}

		for _, log := range logs {
			err := o.upsertBroadcastForListener(tx, log, jobID, jobIDV2)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (o *orm) upsertBroadcastForListener(db *gorm.DB, log types.Log, jobID *models.ID, jobIDV2 int32) error {
	if jobID == nil {
		return db.Exec(`
            INSERT INTO log_broadcasts (block_hash, block_number, log_index, job_id_v2, consumed, created_at)
            VALUES (?, ?, ?, ?, false, NOW())
            ON CONFLICT (job_id_v2, block_hash, log_index) DO UPDATE SET (
                block_hash,
                block_number,
                log_index,
                job_id_v2
            ) = (
                EXCLUDED.block_hash,
                EXCLUDED.block_number,
                EXCLUDED.log_index,
                EXCLUDED.job_id_v2
            )
        `, log.BlockHash, log.BlockNumber, log.Index, jobIDV2).Error
	} else {
		return db.Exec(`
            INSERT INTO log_broadcasts (block_hash, block_number, log_index, job_id, consumed, created_at)
            VALUES (?, ?, ?, ?, false, NOW())
            ON CONFLICT (job_id, block_hash, log_index) DO UPDATE SET (
                block_hash,
                block_number,
                log_index,
                job_id
            ) = (
                EXCLUDED.block_hash,
                EXCLUDED.block_number,
                EXCLUDED.log_index,
                EXCLUDED.job_id
            )
        `, log.BlockHash, log.BlockNumber, log.Index, jobID).Error
	}
}

func (o *orm) WasBroadcastConsumed(blockHash common.Hash, logIndex uint, jobID *models.ID, jobIDV2 int32) (bool, error) {
	var consumed struct{ Consumed bool }
	var err error
	if jobID == nil {
		err = o.db.Raw(`
            SELECT consumed FROM log_broadcasts
            WHERE block_hash = ?
            AND log_index = ?
            AND job_id IS NULL
            AND job_id_v2 = ?
        `, blockHash, logIndex, jobIDV2).Scan(&consumed).Error
	} else {
		err = o.db.Raw(`
            SELECT consumed FROM log_broadcasts
            WHERE block_hash = ?
            AND log_index = ?
            AND job_id = ?
            AND job_id_v2 IS NULL
        `, blockHash, logIndex, jobID).Scan(&consumed).Error
	}

	return consumed.Consumed, err
}

func (o *orm) MarkBroadcastConsumed(blockHash common.Hash, logIndex uint, jobID *models.ID, jobIDV2 int32) error {
	var query *gorm.DB
	if jobID == nil {
		query = o.db.Exec(`
            UPDATE log_broadcasts SET consumed = true
            WHERE block_hash = ?
            AND log_index = ?
            AND job_id IS NULL
            AND job_id_v2 = ?
        `, blockHash, logIndex, jobIDV2)
	} else {
		query = o.db.Exec(`
            UPDATE log_broadcasts SET consumed = true
            WHERE block_hash = ?
            AND log_index = ?
            AND job_id = ?
            AND job_id_v2 IS NULL
        `, blockHash, logIndex, jobID)
	}
	if query.Error != nil {
		return query.Error
	} else if query.RowsAffected == 0 {
		return errors.Errorf("cannot mark log broadcast as consumed: does not exist")
	}
	return nil
}

func (o *orm) UnconsumedLogsPriorToBlock(blockNumber uint64) ([]types.Log, error) {
	logs, err := FetchLogs(o.db, `
        SELECT eth_logs.*, bool_and(log_broadcasts.consumed) as consumed FROM eth_logs
        LEFT JOIN log_broadcasts ON eth_logs.block_hash = log_broadcasts.block_hash AND eth_logs.index = log_broadcasts.log_index
        WHERE eth_logs.block_number < ?
        GROUP BY eth_logs.block_hash, eth_logs.index, log_broadcasts.consumed
        HAVING consumed = false
        ORDER BY eth_logs.order_received, eth_logs.block_number, eth_logs.index ASC
    `, blockNumber)
	if err != nil {
		logger.Errorw("could not fetch logs to broadcast", "error", err)
		return nil, err
	}
	return logs, nil
}

func (o *orm) DeleteLogAndBroadcasts(blockHash common.Hash, logIndex uint) error {
	return o.db.Exec(`DELETE FROM eth_logs WHERE block_hash = ? AND index = ?`, blockHash, logIndex).Error
}

func (o *orm) DeleteUnconsumedBroadcastsForListener(jobID *models.ID, jobIDV2 int32) error {
	if jobID == nil {
		return o.db.Exec(`DELETE FROM log_broadcasts WHERE job_id IS NULL AND job_id_v2 = ? AND consumed = false`, jobIDV2).Error
	} else {
		return o.db.Exec(`DELETE FROM log_broadcasts WHERE job_id = ? AND job_id_v2 IS NULL AND consumed = false`, jobID).Error
	}
}

type logRow struct {
	Address     common.Address
	Topics      pq.ByteaArray
	Data        []byte
	BlockNumber uint64
	BlockHash   common.Hash
	Index       uint
	Removed     bool
}

func FetchLogs(db *gorm.DB, query string, args ...interface{}) ([]types.Log, error) {
	var logRows []logRow
	err := db.Raw(query, args...).Scan(&logRows).Error
	if err != nil {
		return nil, err
	}
	logs := make([]types.Log, len(logRows))
	for i, log := range logRows {
		topics := make([]common.Hash, len(log.Topics))
		bytesTopics := [][]byte(log.Topics)
		for j, topic := range bytesTopics {
			topics[j] = common.BytesToHash(topic)
		}
		logs[i] = types.Log{
			Address:     log.Address,
			Topics:      topics,
			Data:        log.Data,
			BlockNumber: log.BlockNumber,
			BlockHash:   log.BlockHash,
			Index:       log.Index,
			Removed:     log.Removed,
		}
	}
	return logs, nil
}
