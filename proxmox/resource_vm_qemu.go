package proxmox

import (
	"fmt"
	"log"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	pxapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVmQemu() *schema.Resource {
	*pxapi.Debug = true
	return &schema.Resource{
		Create: resourceVmQemuCreate,
		Read:   resourceVmQemuRead,
		Update: resourceVmQemuUpdate,
		Delete: resourceVmQemuDelete,
		Importer: &schema.ResourceImporter{
			State: resourceVmQemuImport,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"desc": {
				Type:     schema.TypeString,
				Optional: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"target_node": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"onboot": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"iso": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"clone": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"qemu_os": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "l26",
			},
			"memory": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"cores": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"sockets": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"network": &schema.Schema{
				Type:          schema.TypeSet,
				Optional:      true,
				ConflictsWith: []string{"nic", "bridge", "vlan", "mac"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},
						"model": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"macaddr": &schema.Schema{
							// TODO: Find a way to set MAC address in .tf config.
							Type:     schema.TypeString,
							Computed: true,
						},
						"bridge": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Default:  "nat",
						},
						"tag": &schema.Schema{
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "VLAN tag.",
							Default:     -1,
						},
						"firewall": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"rate": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							Default:  -1,
						},
						"queues": &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							Default:  -1,
						},
						"link_down": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
			},
			"disk": &schema.Schema{
				Type:          schema.TypeSet,
				Optional:      true,
				ConflictsWith: []string{"disk_gb", "storage", "storage_type"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},
						"type": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"storage": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"storage_type": &schema.Schema{
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "dir",
							Description: "One of PVE types as described: https://pve.proxmox.com/wiki/Storage",
						},
						"size": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"format": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Default:  "raw",
						},
						"cache": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Default:  "none",
						},
						"backup": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"iothread": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"replicate": &schema.Schema{
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
			},
			// Deprecated single disk config.
			"disk_gb": {
				Type:       schema.TypeFloat,
				Deprecated: "Use `disk.size` instead",
				Optional:   true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// bigger ok
					oldf, _ := strconv.ParseFloat(old, 64)
					newf, _ := strconv.ParseFloat(new, 64)
					return oldf >= newf
				},
			},
			"storage": {
				Type:       schema.TypeString,
				Deprecated: "Use `disk.storage` instead",
				Optional:   true,
			},
			"storage_type": {
				Type:       schema.TypeString,
				Deprecated: "Use `disk.type` instead",
				Optional:   true,
				ForceNew:   false,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if new == "" {
						return true // empty template ok
					}
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			// Deprecated single nic config.
			"nic": {
				Type:       schema.TypeString,
				Deprecated: "Use `network` instead",
				Optional:   true,
			},
			"bridge": {
				Type:       schema.TypeString,
				Deprecated: "Use `network.bridge` instead",
				Optional:   true,
			},
			"vlan": {
				Type:       schema.TypeInt,
				Deprecated: "Use `network.tag` instead",
				Optional:   true,
				Default:    -1,
			},
			"mac": {
				Type:       schema.TypeString,
				Deprecated: "Use `network.macaddr` to access the auto generated MAC address",
				Optional:   true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if new == "" {
						return true // macaddr auto-generates and its ok
					}
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"os_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"os_network_config": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"ssh_forward_ip": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"ssh_user": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"ssh_private_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"force_create": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"ci_wait": { // how long to wait before provision
				Type:     schema.TypeInt,
				Optional: true,
				Default:  30,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if old == "" {
						return true // old empty ok
					}
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"ciuser": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"cipassword": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"searchdomain": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"nameserver": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"sshkeys": {
				Type:     schema.TypeString,
				Optional: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
			"ipconfig0": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"ipconfig1": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"preprovision": {
				Type:          schema.TypeBool,
				Optional:      true,
				Default:       true,
				ConflictsWith: []string{"ssh_forward_ip", "ssh_user", "ssh_private_key", "os_type", "os_network_config"},
			},
		},
	}
}

var rxIPconfig = regexp.MustCompile("ip6?=([0-9a-fA-F:\\.]+)")

func resourceVmQemuCreate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	pmParallelBegin(pconf)
	client := pconf.Client
	vmName := d.Get("name").(string)
	networks := d.Get("network").(*schema.Set)
	qemuNetworks := devicesSetToMap(networks)
	disks := d.Get("disk").(*schema.Set)
	qemuDisks := devicesSetToMap(disks)

	config := pxapi.ConfigQemu{
		Name:         vmName,
		Description:  d.Get("desc").(string),
		Onboot:       d.Get("onboot").(bool),
		Memory:       d.Get("memory").(int),
		QemuCores:    d.Get("cores").(int),
		QemuSockets:  d.Get("sockets").(int),
		QemuOs:       d.Get("qemu_os").(string),
		QemuNetworks: qemuNetworks,
		QemuDisks:    qemuDisks,
		// Cloud-init.
		CIuser:       d.Get("ciuser").(string),
		CIpassword:   d.Get("cipassword").(string),
		Searchdomain: d.Get("searchdomain").(string),
		Nameserver:   d.Get("nameserver").(string),
		Sshkeys:      d.Get("sshkeys").(string),
		Ipconfig0:    d.Get("ipconfig0").(string),
		Ipconfig1:    d.Get("ipconfig1").(string),
		// Deprecated single disk config.
		Storage:  d.Get("storage").(string),
		DiskSize: d.Get("disk_gb").(float64),
		// Deprecated single nic config.
		QemuNicModel: d.Get("nic").(string),
		QemuBrige:    d.Get("bridge").(string),
		QemuVlanTag:  d.Get("vlan").(int),
		QemuMacAddr:  d.Get("mac").(string),
	}
	log.Print("[DEBUG] checking for duplicate name")
	dupVmr, _ := client.GetVmRefByName(vmName)

	forceCreate := d.Get("force_create").(bool)
	targetNode := d.Get("target_node").(string)

	if dupVmr != nil && forceCreate {
		pmParallelEnd(pconf)
		return fmt.Errorf("Duplicate VM name (%s) with vmId: %d. Set force_create=false to recycle", vmName, dupVmr.VmId())
	} else if dupVmr != nil && dupVmr.Node() != targetNode {
		pmParallelEnd(pconf)
		return fmt.Errorf("Duplicate VM name (%s) with vmId: %d on different target_node=%s", vmName, dupVmr.VmId(), dupVmr.Node())
	}

	vmr := dupVmr

	if vmr == nil {
		// get unique id
		nextid, err := nextVmId(pconf)
		if err != nil {
			pmParallelEnd(pconf)
			return err
		}
		vmr = pxapi.NewVmRef(nextid)

		vmr.SetNode(targetNode)
		// check if ISO or clone
		if d.Get("clone").(string) != "" {
			sourceVmr, err := client.GetVmRefByName(d.Get("clone").(string))
			if err != nil {
				pmParallelEnd(pconf)
				return err
			}
			log.Print("[DEBUG] cloning VM")
			err = config.CloneVm(sourceVmr, vmr, client)
			if err != nil {
				pmParallelEnd(pconf)
				return err
			}

			// give sometime to proxmox to catchup
			time.Sleep(5 * time.Second)

			err = prepareDiskSize(client, vmr, qemuDisks)
			if err != nil {
				pmParallelEnd(pconf)
				return err
			}

		} else if d.Get("iso").(string) != "" {
			config.QemuIso = d.Get("iso").(string)
			err := config.CreateVm(vmr, client)
			if err != nil {
				pmParallelEnd(pconf)
				return err
			}
		}
	} else {
		log.Printf("[DEBUG] recycling VM vmId: %d", vmr.VmId())

		client.StopVm(vmr)

		err := config.UpdateConfig(vmr, client)
		if err != nil {
			pmParallelEnd(pconf)
			return err
		}

		// give sometime to proxmox to catchup
		time.Sleep(5 * time.Second)

		err = prepareDiskSize(client, vmr, qemuDisks)
		if err != nil {
			pmParallelEnd(pconf)
			return err
		}
	}
	d.SetId(resourceId(targetNode, "qemu", vmr.VmId()))

	// give sometime to proxmox to catchup
	time.Sleep(5 * time.Second)

	log.Print("[DEBUG] starting VM")
	_, err := client.StartVm(vmr)
	if err != nil {
		pmParallelEnd(pconf)
		return err
	}

	// Apply pre-provision if enabled.
	preprovision(d, pconf, client, vmr, true)

	return nil
}

func resourceVmQemuUpdate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	pmParallelBegin(pconf)
	client := pconf.Client
	vmr, err := client.GetVmRefByName(d.Get("name").(string))
	if err != nil {
		pmParallelEnd(pconf)
		return err
	}
	vmName := d.Get("name").(string)
	configDisksSet := d.Get("disk").(*schema.Set)
	qemuDisks := devicesSetToMap(configDisksSet)
	configNetworksSet := d.Get("network").(*schema.Set)
	qemuNetworks := devicesSetToMap(configNetworksSet)

	config := pxapi.ConfigQemu{
		Name:         vmName,
		Description:  d.Get("desc").(string),
		Onboot:       d.Get("onboot").(bool),
		Memory:       d.Get("memory").(int),
		QemuCores:    d.Get("cores").(int),
		QemuSockets:  d.Get("sockets").(int),
		QemuOs:       d.Get("qemu_os").(string),
		QemuNetworks: qemuNetworks,
		QemuDisks:    qemuDisks,
		// Cloud-init.
		CIuser:       d.Get("ciuser").(string),
		CIpassword:   d.Get("cipassword").(string),
		Searchdomain: d.Get("searchdomain").(string),
		Nameserver:   d.Get("nameserver").(string),
		Sshkeys:      d.Get("sshkeys").(string),
		Ipconfig0:    d.Get("ipconfig0").(string),
		Ipconfig1:    d.Get("ipconfig1").(string),
		// Deprecated single disk config.
		Storage:  d.Get("storage").(string),
		DiskSize: d.Get("disk_gb").(float64),
		// Deprecated single nic config.
		QemuNicModel: d.Get("nic").(string),
		QemuBrige:    d.Get("bridge").(string),
		QemuVlanTag:  d.Get("vlan").(int),
		QemuMacAddr:  d.Get("mac").(string),
	}

	err = config.UpdateConfig(vmr, client)
	if err != nil {
		pmParallelEnd(pconf)
		return err
	}

	// give sometime to proxmox to catchup
	time.Sleep(5 * time.Second)

	prepareDiskSize(client, vmr, qemuDisks)

	// give sometime to proxmox to catchup
	time.Sleep(5 * time.Second)

	// Start VM only if it wasn't running.
	vmState, err := client.GetVmState(vmr)
	if err == nil && vmState["status"] == "stopped" {
		log.Print("[DEBUG] starting VM")
		_, err = client.StartVm(vmr)
	} else if err != nil {
		pmParallelEnd(pconf)
		return err
	}

	// Apply pre-provision if enabled.
	preprovision(d, pconf, client, vmr, false)

	pmParallelEnd(pconf)

	// give sometime to bootup
	time.Sleep(9 * time.Second)
	return nil
}

func resourceVmQemuRead(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	pmParallelBegin(pconf)
	client := pconf.Client
	vmr, err := client.GetVmRefByName(d.Get("name").(string))
	if err != nil {
		return err
	}
	config, err := pxapi.NewConfigQemuFromApi(vmr, client)
	if err != nil {
		pmParallelEnd(pconf)
		return err
	}
	d.SetId(resourceId(vmr.Node(), "qemu", vmr.VmId()))
	d.Set("target_node", vmr.Node())
	d.Set("name", config.Name)
	d.Set("desc", config.Description)
	d.Set("onboot", config.Onboot)
	d.Set("memory", config.Memory)
	d.Set("cores", config.QemuCores)
	d.Set("sockets", config.QemuSockets)
	d.Set("qemu_os", config.QemuOs)
	// Cloud-init.
	d.Set("ciuser", config.CIuser)
	d.Set("cipassword", config.CIpassword)
	d.Set("searchdomain", config.Searchdomain)
	d.Set("nameserver", config.Nameserver)
	d.Set("sshkeys", config.Sshkeys)
	d.Set("ipconfig0", config.Ipconfig0)
	d.Set("ipconfig1", config.Ipconfig1)
	// Disks.
	configDisksSet := d.Get("disk").(*schema.Set)
	activeDisksSet := updateDevicesSet(configDisksSet, config.QemuDisks)
	d.Set("disk", activeDisksSet)
	// Networks.
	configNetworksSet := d.Get("network").(*schema.Set)
	activeNetworksSet := updateDevicesSet(configNetworksSet, config.QemuNetworks)
	d.Set("network", activeNetworksSet)
	// Deprecated single disk config.
	d.Set("storage", config.Storage)
	d.Set("disk_gb", config.DiskSize)
	d.Set("storage_type", config.StorageType)
	// Deprecated single nic config.
	d.Set("nic", config.QemuNicModel)
	d.Set("bridge", config.QemuBrige)
	d.Set("vlan", config.QemuVlanTag)
	d.Set("mac", config.QemuMacAddr)

	pmParallelEnd(pconf)
	return nil
}

func resourceVmQemuImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// TODO: research proper import
	err := resourceVmQemuRead(d, meta)
	return []*schema.ResourceData{d}, err
}

func resourceVmQemuDelete(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	pmParallelBegin(pconf)
	client := pconf.Client
	vmId, _ := strconv.Atoi(path.Base(d.Id()))
	vmr := pxapi.NewVmRef(vmId)
	_, err := client.StopVm(vmr)
	if err != nil {
		pmParallelEnd(pconf)
		return err
	}
	// give sometime to proxmox to catchup
	time.Sleep(2 * time.Second)
	_, err = client.DeleteVm(vmr)
	pmParallelEnd(pconf)
	return err
}

// Increase disk size if original disk was smaller than new disk.
func prepareDiskSize(
	client *pxapi.Client,
	vmr *pxapi.VmRef,
	diskConfMap pxapi.QemuDevices,
) error {
	clonedConfig, err := pxapi.NewConfigQemuFromApi(vmr, client)
	for _, diskConf := range diskConfMap {
		diskID := diskConf["id"].(int)
		diskName := fmt.Sprintf("%v%v", diskConf["type"], diskID)

		diskSizeGB := diskConf["size"].(string)
		diskSize, _ := strconv.ParseFloat(strings.Trim(diskSizeGB, "G"), 64)
		if err != nil {
			return err
		}

		if _, diskExists := clonedConfig.QemuDisks[diskID]; !diskExists {
			return err
		}
		clonedDiskSizeGB := clonedConfig.QemuDisks[diskID]["size"].(string)
		clonedDiskSize, _ := strconv.ParseFloat(strings.Trim(clonedDiskSizeGB, "G"), 64)

		if err != nil {
			return err
		}

		diffSize := int(math.Ceil(diskSize - clonedDiskSize))
		if diskSize > clonedDiskSize {
			log.Print("[DEBUG] resizing disk " + diskName)
			_, err = client.ResizeQemuDisk(vmr, diskName, diffSize)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Converting from schema.TypeSet to map of id and conf for each device,
// which will be sent to Proxmox API.
func devicesSetToMap(devicesSet *schema.Set) pxapi.QemuDevices {

	devicesMap := pxapi.QemuDevices{}

	for _, set := range devicesSet.List() {
		setMap, isMap := set.(map[string]interface{})
		if isMap {
			setID := setMap["id"].(int)
			devicesMap[setID] = setMap
		}
	}
	return devicesMap
}

// Update schema.TypeSet with new values comes from Proxmox API.
// TODO: Maybe it's better to create a new Set instead add to current one.
func updateDevicesSet(
	devicesSet *schema.Set,
	devicesMap pxapi.QemuDevices,
) *schema.Set {

	configDevicesMap := devicesSetToMap(devicesSet)
	activeDevicesMap := updateDevicesDefaults(devicesMap, configDevicesMap)

	for _, setConf := range devicesSet.List() {
		devicesSet.Remove(setConf)
		setConfMap := setConf.(map[string]interface{})
		deviceID := setConfMap["id"].(int)
		// Value type should be one of types allowed by Terraform schema types.
		for key, value := range activeDevicesMap[deviceID] {
			// This nested switch is used for nested config like in `net[n]`,
			// where Proxmox uses `key=<0|1>` in string" at the same time
			// a boolean could be used in ".tf" files.
			switch setConfMap[key].(type) {
			case bool:
				switch value.(type) {
				// If the key is bool and value is int (which comes from Proxmox API),
				// should be converted to bool (as in ".tf" conf).
				case int:
					sValue := strconv.Itoa(value.(int))
					bValue, err := strconv.ParseBool(sValue)
					if err == nil {
						setConfMap[key] = bValue
					}
				// If value is bool, which comes from Terraform conf, add it directly.
				case bool:
					setConfMap[key] = value
				}
			// Anything else will be added as it is.
			default:
				setConfMap[key] = value
			}
			devicesSet.Add(setConfMap)
		}
	}

	return devicesSet
}

// Because default values are not stored in Proxmox, so the API returns only active values.
// So to prevent Terraform doing unnecessary diffs, this function reads default values
// from Terraform itself, and fill empty fields.
func updateDevicesDefaults(
	activeDevicesMap pxapi.QemuDevices,
	configDevicesMap pxapi.QemuDevices,
) pxapi.QemuDevices {

	for deviceID, deviceConf := range configDevicesMap {
		if _, ok := activeDevicesMap[deviceID]; !ok {
			activeDevicesMap[deviceID] = configDevicesMap[deviceID]
		}
		for key, value := range deviceConf {
			if _, ok := activeDevicesMap[deviceID][key]; !ok {
				activeDevicesMap[deviceID][key] = value
			}
		}
	}
	return activeDevicesMap
}

// Internal pre-provision.
func preprovision(
	d *schema.ResourceData,
	pconf *providerConfiguration,
	client *pxapi.Client,
	vmr *pxapi.VmRef,
	systemPreProvision bool,
) error {

	if d.Get("preprovision").(bool) {
		log.Print("[DEBUG] setting up SSH forward")
		sshPort, err := pxapi.SshForwardUsernet(vmr, client)
		if err != nil {
			pmParallelEnd(pconf)
			return err
		}

		// Done with proxmox API, end parallel and do the SSH things
		pmParallelEnd(pconf)

		d.SetConnInfo(map[string]string{
			"type":        "ssh",
			"host":        d.Get("ssh_forward_ip").(string),
			"port":        sshPort,
			"user":        d.Get("ssh_user").(string),
			"private_key": d.Get("ssh_private_key").(string),
			"pm_api_url":  client.ApiUrl,
			"pm_user":     client.Username,
			"pm_password": client.Password,
		})

		if systemPreProvision {
			switch d.Get("os_type").(string) {

			case "ubuntu":
				// give sometime to bootup
				time.Sleep(9 * time.Second)
				err = preProvisionUbuntu(d)
				if err != nil {
					return err
				}

			case "centos":
				// give sometime to bootup
				time.Sleep(9 * time.Second)
				err = preProvisionCentos(d)
				if err != nil {
					return err
				}

			default:
				return fmt.Errorf("Unknown os_type: %s", d.Get("os_type").(string))
			}
		}
	}
	return nil
}
