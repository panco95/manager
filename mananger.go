package etcd

import (
	"context"
	"net"
	"strings"
	"time"

	clientV3 "go.etcd.io/etcd/client/v3"
)

type Manager struct {
	etcdPrefix string
	etcd       *clientV3.Client
	nodeChan   chan nodeOperate
	nodes      []*node
	local      *node
}

func NewManager(etcd *clientV3.Client, addr, port, etcdPrefix string) (*Manager, error) {
	var err error

	if addr == "" {
		addr, err = GetOutboundIP()
		if err != nil {
			return nil, err
		}
	}

	addr = addr + ":" + port

	m := &Manager{
		etcdPrefix: etcdPrefix,
		etcd:       etcd,
		nodes:      make([]*node, 0),
		local:      &node{addr: addr},
	}

	m.nodeChan = make(chan nodeOperate)
	go m.nodesWatch()

	if err = m.nodeRegister(false); err != nil {
		return nil, err
	}

	return m, nil
}

type node struct {
	addr string
}

type nodeOperate struct {
	operate string
	addr    string
}

func (m *Manager) GetNodes() []*node {
	return m.nodes
}

func (m *Manager) GetLocalIp() string {
	return m.local.addr
}

func (m *Manager) GetLocalId() string {
	return m.etcdPrefix + "_" + m.local.addr
}

func (m *Manager) nodeRegister(isReconnect bool) error {
	client := m.etcd

	// New lease
	resp, err := client.Grant(context.TODO(), 2)
	if err != nil {
		return err
	}
	// The lease was granted
	if err != nil {
		return err
	}
	_, err = client.Put(context.TODO(), m.GetLocalId(), "0", clientV3.WithLease(resp.ID))
	if err != nil {
		return err
	}
	// keep alive
	ch, err := client.KeepAlive(context.TODO(), resp.ID)
	if err != nil {
		return err
	}
	// monitor etcd connection
	go func() {
		for resp := range ch {
			if resp == nil {
				go m.nodeRegister(true)
				return
			}
		}
	}()

	if !isReconnect {
		go m.serviceWatcher()
		go func() {
			for {
				m.GetAllServices()
				time.Sleep(time.Second * 5)
			}
		}()
	}
	return nil
}

func (m *Manager) serviceWatcher() {
	client := m.etcd

	rch := client.Watch(context.Background(), m.etcdPrefix+"_", clientV3.WithPrefix())
	for wresp := range rch {
		for _, ev := range wresp.Events {
			arr := strings.Split(string(ev.Kv.Key), "_")
			addr := arr[1]
			switch ev.Type {
			case 0:
				m.addNode(addr)
			case 1:
				m.delNode(addr)
			}
		}
	}
}

func (m *Manager) GetAllServices() ([]string, error) {
	client := m.etcd

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	resp, err := client.Get(ctx, m.etcdPrefix+"_", clientV3.WithPrefix())
	cancel()
	if err != nil {
		return []string{}, err
	}

	nodes := make([]string, 0)
	for _, ev := range resp.Kvs {
		arr := strings.Split(string(ev.Key), m.etcdPrefix+"_")
		addr := arr[1]
		m.addNode(addr)
		nodes = append(nodes, addr)
	}

	return nodes, nil
}

func (m *Manager) addNode(addr string) {
	for _, v := range m.nodes {
		if v.addr == addr {
			return
		}
	}

	c := nodeOperate{
		operate: "addNode",
		addr:    addr,
	}
	m.nodeChan <- c
}

func (m *Manager) delNode(addr string) {
	c := nodeOperate{
		operate: "delNode",
		addr:    addr,
	}
	m.nodeChan <- c
}

func (m *Manager) nodesWatch() {
	for c := range m.nodeChan {
		switch c.operate {
		case "addNode":
			m.nodes = append(m.nodes, &node{addr: c.addr})
		case "delNode":
			for i := 0; i < len(m.nodes); i++ {
				if m.nodes[i].addr == c.addr {
					m.nodes = append(m.nodes[:i], m.nodes[i+1:]...)
					i--
				}
			}
		}
	}
}

func GetOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String(), nil
}
