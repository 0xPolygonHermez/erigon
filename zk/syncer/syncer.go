package syncer

type IEtherman interface {
}

type Syncer struct {
	em IEtherman
}

func NewSyncer(etherMan IEtherman) *Syncer {
	return &Syncer{
		em: etherMan,
	}
}
