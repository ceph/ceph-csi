# Encrypted volumes with IBM HPCS or Key Protect

IBM Cloud™ Hyper Protect Crypto Services is a key management and cloud hardware
security module (HSM). It is designed to enable a user to take control of their
cloud data encryption keys and cloud hardware security models. To support this
KMS integration in Ceph CSI and thus enable the HPCS users to make use of the
same in volume encrypted operations, below things are considered.

## Connection to IBM HPCS/Key Protect service

Below parameters/values can be used to establish the connection to the HPCS
service from the CSI driver and to make use of the encryption operations:

```text
* IBM_KP_BASE_URL
The Key Protect/HPCS connection URL.

* IBM_KP_TOKEN_URL
The Token Authentication URL of KeyProtect/HPCS service.

* KMS_SERVICE_NAME
A unique name for the key management service within the project.

* IBM_KP_SERVICE_INSTANCE_ID
The Instance ID of the IBM HPCS service, ex:  crn:v1:bluemix:public:hs-crypto:us-south:a/5d19cf8b82874c2dab37e397426fbc42:e2ae65ff-954b-453f-b0d7-fc5064c203ce::

* IBM_KP_SERVICE_API_KEY
Ex:  06x6DbTkVQ-qCRmq9cK-p9xOQpU2UwJMcdjnIDdr0g2R

* IBM_KP_CUSTOMER_ROOT_KEY
Ex: c7a9aa91-5cb5-48da-a821-e85c27b99d92

* IBM_KP_REGION
Region of the Key Protect service, ex: us-south-2
```

### Values provided in the connection Secret

Considering `SERVICE_API_KEY` and `CUSTOMER_ROOT_KEY` are sensitive information,
those will be provided as a Kubernetes Secret to the CSI driver. The Ceph CSI
KMS plugin interface for the Key Protect will read the Secret name from the kms
ConfigMap and fetch these values. `SESSION_TOKEN and CRK_ARN` values can also be
provided by the user as part of the Secret if needed. How-ever these values are
considered to be optional.

### Values provided in the config map

`SERVICE_INSTANCE_ID` is part of the KMS ConfigMap and there could be an
optional value provided in the ConfigMap for `REGION` too.

### Storage class values or configuration

As like other KMS enablement, the Storage class has to be enabled for encryption
and `encryptionKMSID` has to be provided which is the matching value in the kms
config map to `KMS_SERVICE_NAME`.

## Volume Encrypt or Decrypt Operation

The IBM Key Protect server's `wrap` and `unwrap` functionalities will be used by
the Ceph CSI driver to achieve encryption and decryption of volumes. The DEK can
be wrapped with the help of Customer Root Key (CRK) and can be used for LUKS
operation. The wrapped cipher blob will be stored inside the image metadata ( as
in other KMS integration, ex: AWS). At time of decrypt the DEK will be unwrapped
with the help of cipher blob and Key Protect server

## Integration APIS

[Key Protect Go Client](https://github.com/IBM/keyprotect-go-client) provide the
client SDK to interact with the Key Protect server and perform Key Protect
operations.

## Additional Reference

[Key Protect Doc](https://cloud.ibm.com/docs/key-protect)
