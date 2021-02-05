package main

import (
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink/core/assets"
	clnull "github.com/smartcontractkit/chainlink/core/null"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/chainlink/core/utils"
	"gopkg.in/guregu/null.v4"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//type RunResult struct {
//	ID           int64       `json:"-" gorm:"primary_key;auto_increment"`
//	Data         JSON        `json:"data" gorm:"type:text"`
//	ErrorMessage null.String `json:"error"`
//	CreatedAt    time.Time   `json:"-"`
//	UpdatedAt    time.Time   `json:"-"`
//}
//
type RunStatus string

type Initiator struct {
	ID        int64     `json:"id" gorm:"primary_key;auto_increment"`
	JobSpecID uuid.UUID `json:"jobSpecId"`

	// Type is one of the Initiator* string constants defined just above.
	Type      string    `json:"type" gorm:"index;not null"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`
	//InitiatorParams `json:"params,omitempty"`
	DeletedAt null.Time `json:"-" gorm:"index"`
	UpdatedAt time.Time `json:"-"`
}

type JobRun struct {
	ID        *models.ID `json:"id" gorm:"type:uuid;primary_key;not null"`
	JobSpecID *models.ID `json:"jobId" gorm:"type:uuid"`
	//Result         RunResult    `json:"result" gorm:"foreignkey:ResultID;association_autoupdate:true;association_autocreate:true"`
	ResultID clnull.Int64 `json:"-"`
	//RunRequest     RunRequest   `json:"-" gorm:"foreignkey:RunRequestID;association_autoupdate:true;association_autocreate:true"`
	RunRequestID clnull.Int64 `json:"-"`
	Status       RunStatus    `json:"status" gorm:"default:'unstarted'"`
	//TaskRuns       []TaskRun    `json:"taskRuns"`
	CreatedAt      time.Time    `json:"createdAt"`
	FinishedAt     null.Time    `json:"finishedAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	Initiator      Initiator    `json:"initiator" gorm:"foreignkey:InitiatorID;association_autoupdate:false;association_autocreate:false"`
	InitiatorID    int64        `json:"-"`
	CreationHeight *utils.Big   `json:"creationHeight"`
	ObservedHeight *utils.Big   `json:"observedHeight"`
	DeletedAt      null.Time    `json:"-"`
	Payment        *assets.Link `json:"payment,omitempty"`
}

func panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{
		DSN: "postgres://postgres:node@localhost:5432/chainlink_test_blah?sslmode=disable",
	}), &gorm.Config{}) // TODO add logger
	panicOnErr(err)
	//id := models.NewID()
	//spid := models.NewID()
	//err = db.Create(&models.JobSpec{
	//	ID:         spid,
	//	Name:       "b",
	//}).Error
	//panicOnErr(err)
	//i := models.Initiator{
	//	JobSpecID:       spid,
	//	Type:            "blah",
	//}
	//err = db.Create(&i).Error
	//panicOnErr(err)
	//jr := JobRun{ID: id,
	//	JobSpecID: spid,
	//	InitiatorID: i.ID,
	//}
	//err  = db.Create(&jr).Error
	//panicOnErr(err)
	var runs []JobRun
	err = db. //preloadJobRuns().
			Where("job_spec_id = ?", "a153ba04-71d3-4c8f-ba5d-4a503e503c05").
			Order("created_at desc").
			Find(&runs).Error
	panicOnErr(err)
	fmt.Println(runs)
}
