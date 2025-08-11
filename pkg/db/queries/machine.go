package queries

import (
	"fmt"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func MachineProjectScoped(project string) func(q r.Term) r.Term {
	return func(q r.Term) r.Term {
		return q.Filter(func(row r.Term) r.Term {
			return row.Field("allocation").Field("project").Eq(project)
		})
	}
}

func MachineFilter(rq *apiv2.MachineQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Uuid != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Uuid)
			})
		}

		if rq.Name != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("name").Eq(*rq.Name)
			})
		}

		if rq.Partition != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("partitionid").Eq(*rq.Partition)
			})
		}

		if rq.Size != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("sizeid").Eq(*rq.Size)
			})
		}

		if rq.Rack != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("rackid").Eq(*rq.Rack)
			})
		}

		if rq.Labels != nil {
			for key, value := range rq.Labels.Labels {
				tag := fmt.Sprintf("%s=%s", key, value)
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("tags").Contains(r.Expr(tag))
				})
			}
		}

		if rq.Allocation != nil {
			alloc := rq.Allocation
			if alloc.Project != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("project").Eq(*alloc.Project)
				})
			}

			if alloc.Name != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("name").Eq(*alloc.Name)
				})
			}

			if alloc.Image != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("imageid").Eq(*alloc.Image)
				})
			}

			if alloc.Hostname != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("hostname").Eq(*alloc.Hostname)
				})
			}

			if alloc.AllocationType != nil {
				roleString, err := enum.GetStringValue(*alloc.AllocationType)
				if err != nil {
					return q
				}
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("role").Eq(roleString)
				})
			}

			if alloc.FilesystemLayout != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("filesystemlayout").Field("id").Eq(*alloc.FilesystemLayout)
				})
			}
		}

		if rq.Network != nil {
			nw := rq.Network
			for _, id := range nw.Networks {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Map(func(nw r.Term) r.Term {
						return nw.Field("networkid")
					}).Contains(r.Expr(id))
				})
			}

			for _, prefix := range nw.Prefixes {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Contains(func(nw r.Term) r.Term {
						return nw.Field("prefixes").Contains(r.Expr(prefix))
					})
				})
			}

			for _, ip := range nw.Ips {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Contains(func(nw r.Term) r.Term {
						return nw.Field("ips").Contains(r.Expr(ip))
					})
				})
			}

			for _, destPrefix := range nw.DestinationPrefixes {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Contains(func(nw r.Term) r.Term {
						return nw.Field("destinationprefixes").Contains(r.Expr(destPrefix))
					})
				})
			}

			for _, vrf := range nw.Vrfs {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Contains(func(nw r.Term) r.Term {
						return nw.Field("vrf").Eq(r.Expr(vrf))
					})
				})
			}

			for _, asn := range nw.Asns {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("allocation").Field("networks").Map(func(nw r.Term) r.Term {
						return nw.Field("asn")
					}).Contains(r.Expr(asn))
				})
			}
		}

		if rq.Hardware != nil {
			hw := rq.Hardware

			if hw.Memory != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("memory").Eq(*hw.Memory)
				})
			}

			if hw.CpuCores != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("cpus").Field("cores").Eq(*hw.CpuCores)
				})
			}
		}

		if rq.Nic != nil {
			nic := rq.Nic
			for _, mac := range nic.Macs {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Map(func(nic r.Term) r.Term {
						return nic.Field("macAddress")
					}).Contains(r.Expr(mac))
				})
			}

			for _, name := range nic.Names {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Map(func(nic r.Term) r.Term {
						return nic.Field("name")
					}).Contains(r.Expr(name))
				})
			}

			for _, vrf := range nic.Vrfs {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Map(func(nic r.Term) r.Term {
						return nic.Field("vrf")
					}).Contains(r.Expr(vrf))
				})
			}

			for _, mac := range nic.NeighborMacs {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Contains(func(nic r.Term) r.Term {
						return nic.Field("neighbors").Contains(func(neigh r.Term) r.Term {
							return neigh.Field("macAddress").Eq(mac)
						})
					})
				})
			}

			for _, name := range nic.NeighborNames {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Contains(func(nic r.Term) r.Term {
						return nic.Field("neighbors").Contains(func(neigh r.Term) r.Term {
							return neigh.Field("name").Eq(name)
						})
					})
				})
			}

			for _, vrf := range nic.NeighborVrfs {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("network_interfaces").Contains(func(nic r.Term) r.Term {
						return nic.Field("neighbors").Contains(func(neigh r.Term) r.Term {
							return neigh.Field("vrf").Eq(vrf)
						})
					})
				})
			}
		}

		if rq.Disk != nil {
			disk := rq.Disk
			for _, name := range disk.Names {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("block_devices").Map(func(bd r.Term) r.Term {
						return bd.Field("name")
					}).Contains(r.Expr(name))
				})
			}

			for _, size := range disk.Sizes {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("hardware").Field("block_devices").Map(func(bd r.Term) r.Term {
						return bd.Field("size")
					}).Contains(r.Expr(size))
				})
			}
		}

		if rq.State != nil {
			stateString, err := enum.GetStringValue(rq.State)
			if err != nil {
				return q
			}
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("state").Field("value").Eq(stateString)
			})
		}

		if rq.Ipmi != nil {
			ipmi := rq.Ipmi
			if ipmi.Address != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("address").Eq(*ipmi.Address)
				})
			}

			if ipmi.Mac != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("mac").Eq(*ipmi.Mac)
				})
			}

			if ipmi.User != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("user").Eq(*ipmi.User)
				})
			}

			if ipmi.Interface != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("interface").Eq(*ipmi.Interface)
				})
			}
		}

		if rq.Fru != nil {
			fru := rq.Fru

			if fru.ChassisPartNumber != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("chassis_part_number").Eq(*fru.ChassisPartNumber)
				})
			}

			if fru.ChassisPartSerial != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("chassis_part_serial").Eq(*fru.ChassisPartSerial)
				})
			}

			if fru.BoardMfg != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("board_mfg").Eq(*fru.BoardMfg)
				})
			}

			if fru.BoardSerial != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("board_mfg_serial").Eq(*fru.BoardSerial)
				})
			}

			if fru.BoardPartNumber != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("board_part_number").Eq(*fru.BoardPartNumber)
				})
			}

			if fru.ProductManufacturer != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("product_manufacturer").Eq(*fru.ProductManufacturer)
				})
			}

			if fru.ProductPartNumber != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("product_part_number").Eq(*fru.ProductPartNumber)
				})
			}

			if fru.ProductSerial != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("ipmi").Field("fru").Field("product_serial").Eq(*fru.ProductSerial)
				})
			}
		}
		return q
	}
}
