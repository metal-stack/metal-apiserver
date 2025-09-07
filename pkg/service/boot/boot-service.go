package boot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type bootServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) infrav2connect.BootServiceHandler {
	return &bootServiceServer{
		log:  c.Log.WithGroup("bootService"),
		repo: c.Repo,
	}
}

// Boot implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Boot(context.Context, *connect.Request[infrav2.BootServiceBootRequest]) (*connect.Response[infrav2.BootServiceBootResponse], error) {
	panic("unimplemented")
}

// Dhcp implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Dhcp(context.Context, *connect.Request[infrav2.BootServiceDhcpRequest]) (*connect.Response[infrav2.BootServiceDhcpResponse], error) {
	panic("unimplemented")
}

// Register implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Register(ctx context.Context, rq *connect.Request[infrav2.BootServiceRegisterRequest]) (*connect.Response[infrav2.BootServiceRegisterResponse], error) {
	req := rq.Msg

	if req.Uuid == "" {
		return nil, errors.New("uuid is empty")
	}

	m, err := b.repo.UnscopedMachine().Get(ctx, req.Uuid)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}

	if req.Hardware == nil {
		return nil, errors.New("hardware is nil")
	}
	if req.Bios == nil {
		return nil, errors.New("bios is nil")
	}

	disks := []metal.BlockDevice{}
	for i := range req.Hardware.Disks {
		d := req.Hardware.Disks[i]
		disks = append(disks, metal.BlockDevice{
			Name: d.Name,
			Size: d.Size,
		})
	}

	nics := metal.Nics{}
	for i := range req.Hardware.Nics {
		nic := req.Hardware.Nics[i]
		neighs := metal.Nics{}
		for j := range nic.Neighbors {
			neigh := nic.Neighbors[j]
			neighs = append(neighs, metal.Nic{
				Name:       neigh.Name,
				MacAddress: metal.MacAddress(neigh.Mac),
				// Hostname:   neigh.Hostname, // FIXME do we really have hostname of the neighbour from the metal-hammer ?
				Identifier: neigh.Identifier,
			})
		}
		nics = append(nics, metal.Nic{
			Name:       nic.Name,
			MacAddress: metal.MacAddress(nic.Mac),
			Identifier: nic.Identifier,
			Neighbors:  neighs,
		})
	}

	cpus := []metal.MetalCPU{}
	for _, cpu := range req.Hardware.Cpus {
		cpus = append(cpus, metal.MetalCPU{
			Vendor:  cpu.Vendor,
			Model:   cpu.Model,
			Cores:   cpu.Cores,
			Threads: cpu.Threads,
		})
	}

	gpus := []metal.MetalGPU{}
	for _, gpu := range req.Hardware.Gpus {
		gpus = append(gpus, metal.MetalGPU{
			Vendor: gpu.Vendor,
			Model:  gpu.Model,
		})
	}

	machineHardware := metal.MachineHardware{
		Memory:    req.Hardware.Memory,
		Disks:     disks,
		Nics:      nics,
		MetalCPUs: cpus,
		MetalGPUs: gpus,
	}

	size, err := b.repo.Size().AdditionalMethods().FromHardware(machineHardware)
	if err != nil {
		size = &metal.Size{
			Base: metal.Base{
				ID:   "unknown",
				Name: "unknown",
			},
		}
		b.log.Error("no size found for hardware, defaulting to unknown size", "hardware", machineHardware, "error", err)
	}

	var ipmi metal.IPMI
	if req.Ipmi != nil {
		i := req.Ipmi

		ipmi = metal.IPMI{
			Address:     i.Address,
			MacAddress:  i.Mac,
			User:        i.User,
			Password:    i.Password,
			Interface:   i.Interface,
			BMCVersion:  i.BmcVersion,
			PowerState:  i.PowerState,
			LastUpdated: time.Now(),
		}
		if i.Fru != nil {
			f := i.Fru
			fru := metal.Fru{}
			if f.ChassisPartNumber != nil {
				fru.ChassisPartNumber = *f.ChassisPartNumber
			}
			if f.ChassisPartSerial != nil {
				fru.ChassisPartSerial = *f.ChassisPartSerial
			}
			if f.BoardMfg != nil {
				fru.BoardMfg = *f.BoardMfg
			}
			if f.BoardMfgSerial != nil {
				fru.BoardMfgSerial = *f.BoardMfgSerial
			}
			if f.BoardPartNumber != nil {
				fru.BoardPartNumber = *f.BoardPartNumber
			}
			if f.ProductManufacturer != nil {
				fru.ProductManufacturer = *f.ProductManufacturer
			}
			if f.ProductPartNumber != nil {
				fru.ProductPartNumber = *f.ProductPartNumber
			}
			if f.ProductSerial != nil {
				fru.ProductSerial = *f.ProductSerial
			}
			ipmi.Fru = fru
		}

	}

	if m == nil {
		// machine is not in the database, create it
		m = &metal.Machine{
			Base: metal.Base{
				ID: req.Uuid,
			},
			Allocation: nil,
			SizeID:     size.ID,
			Hardware:   machineHardware,
			BIOS: metal.BIOS{
				Version: req.Bios.Version,
				Vendor:  req.Bios.Vendor,
				Date:    req.Bios.Date,
			},
			State: metal.MachineState{
				Value:              metal.AvailableState,
				MetalHammerVersion: req.MetalHammerVersion,
			},
			LEDState: metal.ChassisIdentifyLEDState{
				Value:       metal.LEDStateOff,
				Description: "Machine registered",
			},
			Tags:        req.Tags,
			IPMI:        ipmi,
			PartitionID: req.Partition,
		}

		// FIXME this wrong here, we must move this code to the repo and call the datastore with this new machine
		// _, err := b.repo.UnscopedMachine().Create(ctx, m)
		// if err != nil {
		// 	return nil, err
		// }
	} else {
		// machine has already registered, update it
		updatedMachine := *m

		updatedMachine.SizeID = size.ID
		updatedMachine.Hardware = machineHardware
		updatedMachine.BIOS.Version = req.Bios.Version
		updatedMachine.BIOS.Vendor = req.Bios.Vendor
		updatedMachine.BIOS.Date = req.Bios.Date
		updatedMachine.IPMI = ipmi
		updatedMachine.State.MetalHammerVersion = req.MetalHammerVersion
		updatedMachine.PartitionID = req.Partition

		// FIXME this wrong here, we must move this code to the repo and call the datastore with this updated machine infp
		// _, err = b.repo.UnscopedMachine().Update(ctx, m.ID, &updatedMachine)
		// if err != nil {
		// 	return nil, err
		// }
	}

	// FIXME Event and Switch service missing
	// ec, err := b.ds.FindProvisioningEventContainer(m.ID)
	// if err != nil && !metal.IsNotFound(err) {
	// 	return nil, err
	// }

	// if ec == nil {
	// 	err = b.ds.CreateProvisioningEventContainer(&metal.ProvisioningEventContainer{
	// 		Base:       metal.Base{ID: m.ID},
	// 		Liveliness: metal.MachineLivelinessAlive,
	// 	},
	// 	)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// old := *m
	// err = retry.Do(
	// 	func() error {
	// 		// RackID is set here
	// 		err := b.ds.ConnectMachineWithSwitches(m)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		return b.ds.UpdateMachine(&old, m)
	// 	},
	// 	retry.Attempts(10),
	// 	retry.RetryIf(func(err error) bool {
	// 		return metal.IsConflict(err)
	// 	}),
	// 	retry.DelayType(retry.CombineDelay(retry.BackOffDelay, retry.RandomDelay)),
	// 	retry.LastErrorOnly(true),
	// )

	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&infrav2.BootServiceRegisterResponse{
		Uuid:      req.Uuid,
		Size:      size.ID,
		Partition: m.PartitionID,
	}), nil
}

// Report implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Report(context.Context, *connect.Request[infrav2.BootServiceReportRequest]) (*connect.Response[infrav2.BootServiceReportResponse], error) {
	panic("unimplemented")
}

// SuperUserPassword implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) SuperUserPassword(context.Context, *connect.Request[infrav2.BootServiceSuperUserPasswordRequest]) (*connect.Response[infrav2.BootServiceSuperUserPasswordResponse], error) {
	panic("unimplemented")
}

// Wait implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Wait(context.Context, *connect.Request[infrav2.BootServiceWaitRequest], *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	panic("unimplemented")
}
