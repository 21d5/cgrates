/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package agents

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/utils"
	"github.com/miekg/dns"
)

const (
	QueryType   = "QueryType"
	E164Address = "E164"
)

// e164FromNAPTR extracts the E164 address out of a NAPTR name record
func e164FromNAPTR(name string) (e164 string, err error) {
	i := strings.Index(name, ".e164.arpa")
	if i == -1 {
		return "", errors.New("unknown format")
	}
	e164 = utils.ReverseString(
		strings.Replace(name[:i], ".", "", -1))
	return
}

// newDADataProvider constructs a DataProvider for a diameter message
func newDNSDataProvider(req *dns.Msg,
	w dns.ResponseWriter) config.DataProvider {
	return &dnsDP{req: req, w: w,
		cache: config.NewNavigableMap(nil)}
}

// dnsDP implements engien.DataProvider, serving as dns.Msg decoder
// cache is used to cache queries within the message
type dnsDP struct {
	req   *dns.Msg
	w     dns.ResponseWriter
	cache *config.NavigableMap
}

// String is part of engine.DataProvider interface
// when called, it will display the already parsed values out of cache
func (dP *dnsDP) String() string {
	return utils.ToJSON(dP.req)
}

// AsNavigableMap is part of engine.DataProvider interface
func (dP *dnsDP) AsNavigableMap([]*config.FCTemplate) (
	nm *config.NavigableMap, err error) {
	return nil, utils.ErrNotImplemented
}

// FieldAsString is part of engine.DataProvider interface
func (dP *dnsDP) FieldAsString(fldPath []string) (data string, err error) {
	var valIface interface{}
	valIface, err = dP.FieldAsInterface(fldPath)
	if err != nil {
		return
	}
	return utils.IfaceAsString(valIface)
}

// RemoteHost is part of engine.DataProvider interface
func (dP *dnsDP) RemoteHost() net.Addr {
	return utils.NewNetAddr(dP.w.RemoteAddr().Network(), dP.w.RemoteAddr().String())
}

// FieldAsInterface is part of engine.DataProvider interface
func (dP *dnsDP) FieldAsInterface(fldPath []string) (data interface{}, err error) {
	if data, err = dP.cache.FieldAsInterface(fldPath); err != nil {
		if err != utils.ErrNotFound { // item found in cache
			return nil, err
		}
		err = nil // cancel previous err
	} else {
		return // data was found in cache
	}
	data = ""
	// Return Question[0] by default
	if len(dP.req.Question) != 0 {
		data = dP.req.Question[0]
	}
	dP.cache.Set(fldPath, data, false, false)
	return
}

// dnsWriteErr writes the error with code back to the client
func dnsWriteMsg(w dns.ResponseWriter, msg *dns.Msg) (err error) {
	if err = w.WriteMsg(msg); err != nil {
		utils.Logger.Warning(
			fmt.Sprintf("<%s> error: <%s> when writing on connection",
				utils.DNSAgent, err.Error()))
	}
	return
}

// appendDNSAnswer will append the right answer payload to the message
func appendDNSAnswer(msg *dns.Msg) (err error) {
	switch msg.Question[0].Qtype {
	case dns.TypeA:
		msg.Answer = append(msg.Answer,
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   msg.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60},
			},
		)
	default:
		return fmt.Errorf("unsupported DNS type: <%v>", msg.Question[0].Qtype)
	}
	return
}

// updateDNSMsgFromNM will update DNS message with values from NavigableMap
func updateDNSMsgFromNM(msg *dns.Msg, nm *config.NavigableMap) (err error) {
	for _, valX := range nm.Values() {
		nmItms, cast := valX.([]*config.NMItem)
		if !cast {
			return fmt.Errorf("cannot cast val: %s into []*config.NMItem", utils.ToJSON(valX))
		}
		if len(nmItms) == 0 {
			continue
		}
		cfgItm := nmItms[0] // first item gives some config for the rest
		if cfgItm.Config.NewBranch {
			if err = appendDNSAnswer(msg); err != nil {
				return
			}
		}
		if len(cfgItm.Path) == 0 {
			return errors.New("empty path in config item")
		}
		switch cfgItm.Path[0] {
		case utils.MetaResponseCode:
			var itm int64
			if itm, err = utils.IfaceAsInt64(cfgItm.Data); err != nil {
				return fmt.Errorf("item: <%s>, err: %s", cfgItm.Path[0], err.Error())
			}
			msg.Rcode = int(itm)
			continue
		case utils.MetaAnswer:
			lastAnswer := msg.Answer[len(msg.Answer)-1]
			switch msg.Question[0].Qtype {
			case dns.TypeA:
				var ip string
				if ip, err = utils.IfaceAsString(cfgItm.Data); err != nil {
					return
				}
				lastAnswer.(*dns.A).A = net.ParseIP(ip)
			}

		}
	}
	return
}
