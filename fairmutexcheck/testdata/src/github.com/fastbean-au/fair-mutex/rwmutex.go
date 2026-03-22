package fairmutex

type RWMutex struct {}

func New() *RWMutex {return new(RWMutex)}

func (m *RWMutex) Stop() {}
func (m *RWMutex) Lock() {}
func (m *RWMutex) Unlock() {}
